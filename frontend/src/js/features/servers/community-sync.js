import "../../../css/app/community-sync.css";
import {
  ConnectToServer,
  GetCommunityServerStatuses,
  GetRootDirectory,
  PreviewServerMapSync,
  StartServerMapSync,
  RestoreLastServerMapSync,
} from "../../../../wailsjs/go/app/App";
import { BrowserOpenURL } from "../../../../wailsjs/runtime/runtime";
import { getConfig, saveConfig } from "../../core/config.js";
import { switchAppPage } from "../../core/ui-shell.js";
import { COMMUNITY_SERVER_PRESETS, formatSyncBytes } from "./community-presets.js";

const COMMUNITY_STATUS_PAGE = "https://l4d2.fastbot.top/servers";
const mountedViews = new WeakMap();

function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

function button(text, action, primary = false) {
  const node = element("button", primary ? "btn btn-primary" : "btn btn-secondary", text);
  node.type = "button";
  node.addEventListener("click", action);
  return node;
}

function showList(container, title, entries, describe) {
  if (!entries.length) return;
  const group = element("details", "community-sync-list");
  const summary = element("summary", "", `${title}（${entries.length}）`);
  const list = element("ul");
  for (const item of entries) list.appendChild(element("li", "", describe(item)));
  group.append(summary, list);
  container.appendChild(group);
}

function statusPayload(value) {
  if (typeof value === "string") return JSON.parse(value);
  return value;
}

function isVisible(view) {
  return view.closest(".page-view")?.classList.contains("active") && !document.hidden;
}

function createCommunityView(parent) {
  const view = element("section", "community-sync-view");
  const heading = element("div", "community-sync-heading");
  heading.appendChild(element("h3", "", "预设服务器"));
  const statusActions = element("div", "community-sync-actions");
  const refresh = button("刷新状态", () => refreshStatuses());
  statusActions.append(refresh, button("网站状态页", () => BrowserOpenURL(COMMUNITY_STATUS_PAGE)));
  heading.appendChild(statusActions);
  const cards = element("div", "community-server-cards");
  const statusViews = new Map();
  for (const preset of COMMUNITY_SERVER_PRESETS) {
    const card = element("article", "community-server-card");
    const name = element("strong", "", preset.name);
    const state = element("span", "community-server-state", "等待查询");
    const title = element("div", "community-server-title", preset.mode);
    const map = element("div", "community-server-map", preset.address);
    const info = element("div", "community-server-info");
    const actions = element("div", "community-sync-actions");
    const connect = button("连接服务器", async () => {
      connect.disabled = true;
      try { await ConnectToServer(preset.address); }
      catch (error) { state.textContent = String(error?.message || error); state.dataset.state = "error"; }
      finally { connect.disabled = false; }
    }, true);
    actions.appendChild(connect);
    card.append(name, state, title, map, info, actions);
    statusViews.set(preset.id, { state, title, map, info });
    cards.appendChild(card);
  }
  view.append(heading, cards);

  const sync = element("section", "community-map-sync");
  const syncHeading = element("div", "community-sync-heading");
  syncHeading.appendChild(element("h3", "", "同步服务器地图"));
  const settings = button("修改同步服务器", () => switchAppPage("settings"));
  syncHeading.appendChild(settings);
  const source = element("p", "community-sync-source");
  const directory = element("p", "community-sync-directory");
  const note = element("p", "community-sync-note", "补齐缺图，并按服务器明确删除记录隔离旧图。其他 MOD、workshop 子目录和仍在服务器清单里的依赖会保留。");
  const actions = element("div", "community-sync-actions");
  const preview = button("预览地图同步", () => previewSync());
  const apply = button("执行同步", () => applySync(), true);
  apply.disabled = true;
  const restore = button("恢复上次清理", () => restoreSync());
  const downloads = button("查看下载任务", () => switchAppPage("downloads"));
  actions.append(preview, apply, restore, downloads);
  const cleanupLabel = element("label", "community-sync-checkbox");
  const cleanup = element("input");
  cleanup.type = "checkbox";
  cleanup.checked = true;
  cleanupLabel.append(cleanup, document.createTextNode("同时清理明确删除的旧地图（移至可恢复目录）"));
  const message = element("p", "community-sync-message", "选择本机 addons 目录后，先预览待下载与待清理的地图。");
  message.setAttribute("role", "status");
  message.setAttribute("aria-live", "polite");
  const lists = element("div", "community-sync-lists");
  sync.append(syncHeading, source, directory, note, actions, cleanupLabel, message, lists);
  view.appendChild(sync);
  parent.prepend(view);

  let currentPlan = null;
  let busy = false;
  let statusBusy = false;
  let requestSequence = 0;
  let lastStatusAt = 0;

  function setBusy(value) {
    busy = value;
    preview.disabled = value;
    restore.disabled = value;
    settings.disabled = value;
    cleanup.disabled = value;
    apply.disabled = value || !currentPlan;
  }

  function invalidatePlan() {
    currentPlan = null;
    apply.disabled = true;
  }

  function fail(error) {
    message.textContent = String(error?.message || error || "操作失败，请稍后重试");
    message.dataset.state = "error";
  }

  async function updateContext() {
    const configured = getConfig().mapSyncServer || "cd.fastbot.top";
    if (source.dataset.source !== configured) {
      source.dataset.source = configured;
      source.textContent = `同步来源：${configured}（设置 → 网络设置可修改）`;
      invalidatePlan();
    }
    try {
      const root = await GetRootDirectory();
      if (directory.dataset.directory !== root) {
        directory.dataset.directory = root;
        directory.textContent = root ? `本机目录：${root}` : "本机目录：尚未选择 addons 目录";
        invalidatePlan();
      }
    } catch (error) {
      directory.textContent = `本机目录读取失败：${String(error?.message || error)}`;
      invalidatePlan();
    }
  }

  async function refreshStatuses() {
    if (statusBusy) return;
    statusBusy = true;
    refresh.disabled = true;
    const sequence = ++requestSequence;
    try {
      const data = statusPayload(await GetCommunityServerStatuses());
      if (!Array.isArray(data?.servers)) throw new Error("服务器状态数据无效");
      if (sequence !== requestSequence) return;
      for (const preset of COMMUNITY_SERVER_PRESETS) {
        const output = statusViews.get(preset.id);
        const server = data.servers.find((item) => item.id === preset.id);
        const available = server?.online === true;
        output.state.textContent = available ? `在线 · ${Number(server.players) || 0}/${Number(server.max_players) || 0} 人` : "离线或暂时无法查询";
        output.state.dataset.state = available ? "online" : "offline";
        output.title.textContent = available ? (server.server_name || preset.mode) : preset.mode;
        const chapter = server?.chapter && server?.chapter_count ? ` ${server.chapter}/${server.chapter_count}` : "";
        output.map.textContent = available ? `${server.map_title || server.map || "未知地图"}${chapter}` : preset.address;
        output.info.textContent = available ? `${preset.address} · 查询延迟 ${Number(server.ping_ms) || 0} ms` : "连接按钮仍可直接尝试加入";
      }
      lastStatusAt = Date.now();
    } catch (error) {
      for (const preset of COMMUNITY_SERVER_PRESETS) {
        const output = statusViews.get(preset.id);
        output.state.textContent = "状态查询失败";
        output.state.dataset.state = "error";
        output.title.textContent = preset.mode;
        output.map.textContent = preset.address;
        output.info.textContent = String(error?.message || error);
      }
    } finally {
      statusBusy = false;
      refresh.disabled = false;
    }
  }

  async function previewSync() {
    if (busy) return;
    invalidatePlan();
    setBusy(true);
    message.textContent = "正在查询服务器清单、删除记录并比对本机地图…";
    message.dataset.state = "";
    lists.replaceChildren();
    try {
      const plan = await PreviewServerMapSync();
      if (!plan?.id) throw new Error("未获得有效的同步预览");
      currentPlan = plan;
      source.textContent = `同步来源：${plan.server}`;
      directory.textContent = `本机目录：${plan.directory}`;
      const missing = Array.isArray(plan.missing) ? plan.missing : [];
      const cleanupItems = Array.isArray(plan.cleanup) ? plan.cleanup : [];
      const unresolved = Array.isArray(plan.unresolved) ? plan.unresolved : [];
      message.textContent = `已匹配 ${plan.present || 0} 项；待下载 ${missing.length} 项（约 ${formatSyncBytes(plan.bytes)}）；待清理 ${cleanupItems.length} 项；需手动处理 ${unresolved.length} 项。`;
      showList(lists, "待下载", missing, (item) => `${item.title || item.id} · ${formatSyncBytes(item.size_bytes)}`);
      showList(lists, "待清理（可恢复）", cleanupItems, (item) => `${item.name} · ${formatSyncBytes(item.size)}`);
      showList(lists, "缺少明确工坊 ID，需手动处理", unresolved, (item) => item.title || item.id);
    } catch (error) { fail(error); }
    finally { setBusy(false); }
  }

  async function applySync() {
    if (busy || !currentPlan) return;
    const id = currentPlan.id;
    setBusy(true);
    message.textContent = "正在重新核对预览并提交下载任务…";
    try {
      const result = await StartServerMapSync(id, cleanup.checked);
      invalidatePlan();
      const destination = result.quarantine_directory ? `；恢复目录：${result.quarantine_directory}` : "";
      message.textContent = `已提交 ${result.queued || 0} 个下载任务，${result.already_queued || 0} 项已在队列；已隔离 ${result.quarantined || 0} 张地图${destination}。提交不等于下载完成，请在“查看下载任务”中检查结果。`;
      message.dataset.state = "success";
      if (Array.isArray(result.errors) && result.errors.length) {
        message.appendChild(document.createTextNode(` 问题：${result.errors.join("；")}`));
      }
    } catch (error) {
      invalidatePlan();
      fail(error);
    } finally { setBusy(false); }
  }

  async function restoreSync() {
    if (busy) return;
    setBusy(true);
    try {
      const count = await RestoreLastServerMapSync();
      invalidatePlan();
      lists.replaceChildren();
      message.textContent = `已恢复 ${Number(count) || 0} 张地图。可重新预览同步，或返回 MOD 列表刷新。`;
      message.dataset.state = "success";
    } catch (error) { fail(error); }
    finally { setBusy(false); }
  }

  const pageChanged = (event) => {
    if (event.detail?.page !== "servers") return;
    updateContext();
    if (Date.now() - lastStatusAt >= 30000) refreshStatuses();
  };
  document.addEventListener("app:page-change", pageChanged);
  const interval = setInterval(() => {
    if (!view.isConnected) {
      clearInterval(interval);
      document.removeEventListener("app:page-change", pageChanged);
      return;
    }
    if (isVisible(view)) {
      if (Date.now() - lastStatusAt >= 30000) refreshStatuses();
      if (!busy) updateContext();
    }
  }, 30000);
  updateContext();
  if (isVisible(view)) refreshStatuses();
  return { view, updateContext, refreshStatuses };
}

export function mountCommunityServers(list) {
  const parent = list?.closest(".server-scroll-content");
  if (!parent) return;
  const existing = mountedViews.get(parent);
  if (existing?.view.isConnected) {
    existing.updateContext();
    return;
  }
  mountedViews.set(parent, createCommunityView(parent));
}
