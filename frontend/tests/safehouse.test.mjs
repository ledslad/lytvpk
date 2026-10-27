import assert from "node:assert/strict";
import test from "node:test";
import { mountSafehouseView } from "../src/js/features/safehouse/safehouse-view.js";

// Small DOM fixture for exercising the page's async actions without Wails.
class Node {
  constructor(tag = "") {
    this.tagName = tag;
    this.children = [];
    this.dataset = {};
    this.listeners = new Map();
    this.className = "";
    this.disabled = false;
    this.value = "";
    this.text = "";
    this.classList = {
      contains: (name) => this.className.split(" ").includes(name),
      add: (name) => { if (!this.classList.contains(name)) this.className += ` ${name}`; },
    };
  }
  get isConnected() { return this.connected || Boolean(this.parent?.isConnected); }
  get textContent() { return this.text + this.children.map((child) => child.textContent).join(""); }
  set textContent(text) { this.text = text; this.children = []; }
  append(...nodes) { for (const node of nodes) { node.parent = this; this.children.push(node); } }
  appendChild(node) { this.append(node); return node; }
  prepend(node) { node.parent = this; this.children.unshift(node); }
  replaceChildren(...nodes) { this.children = []; this.text = ""; this.append(...nodes); }
  setAttribute(name, value) { this[name] = value; }
  addEventListener(name, callback) { this.listeners.set(name, [...(this.listeners.get(name) || []), callback]); }
  removeEventListener(name, callback) { this.listeners.set(name, (this.listeners.get(name) || []).filter((item) => item !== callback)); }
  closest(selector) { return this.classList.contains(selector.slice(1)) ? this : this.parent?.closest(selector); }
  async emit(name, event = {}) {
    if (name === "click" && this.disabled) return;
    await Promise.all((this.listeners.get(name) || []).map((callback) => callback(event)));
  }
}

function findAll(node, predicate) {
  return [node, ...node.children.flatMap((child) => findAll(child, predicate))].filter(predicate);
}

async function fixture(t) {
  const doc = new Node();
  doc.createElement = (tag) => new Node(tag);
  doc.createElementNS = (_namespace, tag) => new Node(tag);
  doc.createTextNode = (text) => { const node = new Node(); node.textContent = text; return node; };
  doc.hidden = false;
  const previousDocument = globalThis.document;
  globalThis.document = doc;
  t.after(() => { globalThis.document = previousDocument; });
  const timer = { callback: null };
  t.mock.method(globalThis, "setInterval", (callback) => { timer.callback = callback; return 1; });
  t.mock.method(globalThis, "clearInterval", () => {});
  const page = new Node("section");
  page.connected = true;
  page.className = "page-view";
  const parent = new Node("div");
  page.append(parent);
  const state = { config: { mapSyncServer: "cd.fastbot.top" }, root: "D:/Game/addons", started: [], connected: [], statusCalls: 0, cleared: [], confirmation: null };
  const services = {
    getConfig: () => state.config,
    saveConfig: async (patch) => Object.assign(state.config, patch),
    GetRootDirectory: async () => state.root,
    GetCommunityServerStatuses: async () => {
      state.statusCalls++;
      return { servers: [{ id: "cd", online: true, players: 4, max_players: 8, map_title: "Test campaign" }] };
    },
    ConnectToServer: async (address) => state.connected.push(address),
    PreviewServerMapSync: async () => ({
      id: "preview-1", server: state.config.mapSyncServer, directory: state.root, present: 2, bytes: 1024,
      missing: [{ id: "map", title: "Missing campaign", size_bytes: 1024 }],
      cleanup: [{ name: "old.vpk", size: 512 }], unresolved: [],
    }),
    StartServerMapSync: async (...args) => { state.started.push(args); return { queued: 1, quarantined: 0 }; },
    RestoreLastServerMapSync: async () => 1,
    PreviewServerMapSyncTrash: async () => state.trash || ({ id: "trash-1", directory: state.root, maps: 2, files: 4, bytes: 1024, batches: 1, skipped: 0 }),
    ClearServerMapSyncTrash: async (id) => { state.cleared.push(id); return { files: 4, bytes: 1024, errors: [] }; },
    showConfirmModal: (title, message, confirm) => { state.confirmation = { title, message, confirm }; },
    switchAppPage: () => {}, BrowserOpenURL: () => {},
  };
  const mounted = mountSafehouseView(parent, services);
  await mounted.updateContext();
  const button = (label) => findAll(parent, (node) => node.tagName === "button" && node.textContent === label)[0];
  const source = findAll(parent, (node) => node.id === "safehouse-sync-source")[0];
  const message = findAll(parent, (node) => node.className === "community-sync-message")[0];
  return { parent, page, doc, state, services, mounted, button, source, message, timer };
}

test("safehouse requires a preview and keeps cleanup optional with folded details", async (t) => {
  const { parent, state, button, message } = await fixture(t);
  assert.equal(button("同步地图").disabled, true);
  await button("同步地图").emit("click");
  assert.equal(state.started.length, 0);
  await button("预览变更").emit("click");
  assert.equal(button("同步地图").disabled, false);
  const details = findAll(parent, (node) => node.tagName === "details");
  assert.equal(details.length, 2);
  assert.ok(details.every((node) => !node.open));
  const cleanup = findAll(parent, (node) => node.type === "checkbox")[0];
  cleanup.checked = false;
  await button("同步地图").emit("click");
  assert.deepEqual(state.started, [["preview-1", false]]);
  assert.equal(button("同步地图").disabled, true);
  assert.match(message.textContent, /提交不等于下载完成/);
});

test("editing the source invalidates the preview until the new source is saved", async (t) => {
  const { state, source, button } = await fixture(t);
  await button("预览变更").emit("click");
  source.value = "https://example.org";
  await source.emit("input");
  assert.equal(button("同步地图").disabled, true);
  assert.equal(button("预览变更").disabled, true);
  await button("保存").emit("click");
  assert.equal(state.config.mapSyncServer, "example.org");
  assert.equal(button("预览变更").disabled, false);
  assert.equal(button("同步地图").disabled, true);
});

test("changing addons directory invalidates the previous preview", async (t) => {
  const { state, mounted, button } = await fixture(t);
  await button("预览变更").emit("click");
  state.root = "E:/OtherGame/addons";
  await mounted.updateContext();
  assert.equal(button("同步地图").disabled, true);
});

test("restoring isolated maps clears the old plan and reports the restored count", async (t) => {
  const { parent, button, message } = await fixture(t);
  await button("预览变更").emit("click");
  await button("恢复上次清理").emit("click");
  assert.equal(button("同步地图").disabled, true);
  assert.match(message.textContent, /已恢复 1 张地图/);
  assert.equal(findAll(parent, (node) => node.tagName === "details").length, 0);
});

test("emptying the map recycle bin requires explicit confirmation and invalidates the preview", async (t) => {
  const { state, button, message } = await fixture(t);
  await button("预览变更").emit("click");
  await button("清空回收站").emit("click");
  assert.deepEqual(state.cleared, []);
  assert.match(state.confirmation.message, /永久删除 2 个地图 VPK/);
  assert.match(state.confirmation.message, /无法恢复/);
  await state.confirmation.confirm();
  assert.deepEqual(state.cleared, ["trash-1"]);
  assert.equal(button("同步地图").disabled, true);
  assert.match(message.textContent, /已清理 4 个文件/);
});

test("an empty recycle bin does not ask for deletion confirmation", async (t) => {
  const { state, button, message } = await fixture(t);
  state.trash = { id: "empty", batches: 0, files: 0, skipped: 1 };
  await button("清空回收站").emit("click");
  assert.deepEqual(state.cleared, []);
  assert.equal(state.confirmation, null);
  assert.match(message.textContent, /没有可安全清理的文件/);
  assert.match(message.textContent, /已保留 1 个/);
});

test("status refresh belongs to the safehouse page and server connections remain separate", async (t) => {
  const { parent, doc, page, state, timer } = await fixture(t);
  assert.equal(state.statusCalls, 0);
  await doc.emit("app:page-change", { detail: { page: "servers" } });
  assert.equal(state.statusCalls, 0);
  page.className = "page-view active";
  await doc.emit("app:page-change", { detail: { page: "safehouse" } });
  await Promise.resolve();
  assert.equal(state.statusCalls, 1);
  page.className = "page-view";
  timer.callback();
  assert.equal(state.statusCalls, 1);
  const connect = findAll(parent, (node) => node.tagName === "button" && node.textContent === "加入服务器");
  await connect[0].emit("click");
  await connect[1].emit("click");
  assert.deepEqual(state.connected, ["cd.fastbot.top", "gd.fastbot.top"]);
});

test("recycle bin reports skipped batches without claiming success or touching manual disabled files", async (t) => {
  const { state, button, message } = await fixture(t);
  state.trash = { id: "partial", batches: 1, files: 2, maps: 1, skipped: 1, skipped_reasons: ["server-sync-broken: 无效的地图恢复记录"] };
  await button("清空回收站").emit("click");
  assert.deepEqual(state.cleared, []);
  assert.match(state.confirmation.message, /手动禁用的文件不受影响/);
  assert.match(state.confirmation.message, /server-sync-broken/);
  await state.confirmation.confirm();
  assert.equal(message.dataset.state, "error");
  assert.match(message.textContent, /无效的地图恢复记录/);
});
