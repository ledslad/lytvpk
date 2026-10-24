import { COMMUNITY_SERVER_PRESETS, formatSyncBytes, normalizeSyncSource } from "./presets.js";

const COMMUNITY_STATUS_PAGE = "https://l4d2.fastbot.top/servers";
const mountedViews = new WeakMap();

function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

const ICON_PATHS = {
  home: ["m3 10 9-7 9 7", "M5 9v11h14V9", "M9 20v-7h6v7"],
  server: ["M4 4h16v6H4z", "M4 14h16v6H4z", "M8 7h.01", "M8 17h.01"],
  refresh: ["M20 7v5h-5", "M4 17v-5h5", "M6.1 8a7 7 0 0 1 11.6-2L20 8", "M4 16l2.3 2A7 7 0 0 0 18 16"],
  external: ["M15 3h6v6", "m21 3-9 9", "M10 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-5"],
  arrow: ["M4 12h16", "m14 6 6 6-6 6"],
  sync: ["M12 3v12", "m7 10 5 5 5-5", "M4 16v5h16v-5"],
  folder: ["M3 7V5h6l2 2h10v13H3z"],
  restore: ["M3 5v6h6", "M3 11a9 9 0 1 1 2.5 8", "M12 7v5l3 2"],
  trash: ["M3 6h18", "M9 6V3h6v3", "M5 6l1 15h12l1-15", "M10 10v7", "M14 10v7"],
  shield: ["m12 3 8 3v6c0 5-8 9-8 9s-8-4-8-9V6z", "m8 12 3 3 5-6"],
};

function icon(name) {
  const node = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  node.setAttribute("viewBox", "0 0 24 24");
  node.setAttribute("fill", "none");
  node.setAttribute("stroke", "currentColor");
  node.setAttribute("stroke-width", "1.7");
  node.setAttribute("stroke-linecap", "round");
  node.setAttribute("stroke-linejoin", "round");
  node.setAttribute("aria-hidden", "true");
  node.setAttribute("class", "safehouse-icon");
  for (const d of ICON_PATHS[name] || []) {
    const path = document.createElementNS("http://www.w3.org/2000/svg", "path");
    path.setAttribute("d", d);
    node.appendChild(path);
  }
  return node;
}

function withIcon(node, name) {
  node.prepend(icon(name));
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

function createCommunityView(parent, services) {
  const {
    ConnectToServer, GetCommunityServerStatuses, GetRootDirectory,
    PreviewServerMapSync, StartServerMapSync, RestoreLastServerMapSync,
    PreviewServerMapSyncTrash, ClearServerMapSyncTrash, showConfirmModal,
    getConfig, saveConfig, switchAppPage, BrowserOpenURL,
  } = services;
  const view = element("section", "community-sync-view");
  const heading = element("header", "safehouse-page-header");
  const brand = element("div", "safehouse-brand");
  const brandMark = element("div", "safehouse-brand-mark");
  brandMark.appendChild(icon("home"));
  const brandCopy = element("div", "safehouse-brand-copy");
  brandCopy.append(
    element("h2", "", "麻衣安全屋"),
    element("p", "", "加入服务器，让本机地图与服务器保持同步。"),
  );
  brand.append(brandMark, brandCopy);
  const statusActions = element("div", "community-sync-actions safehouse-header-actions");
  const refresh = withIcon(button("刷新状态", () => refreshStatuses()), "refresh");
  statusActions.append(refresh, withIcon(button("网站状态页", () => BrowserOpenURL(COMMUNITY_STATUS_PAGE)), "external"));
  heading.append(brand, statusActions);
  const cards = element("div", "community-server-cards");
  const statusViews = new Map();
  for (const preset of COMMUNITY_SERVER_PRESETS) {
    const card = element("article", "community-server-card");
    const cardHeading = element("div", "community-server-heading");
    const identity = element("div", "community-server-identity");
    const serverMark = element("div", "community-server-symbol");
    serverMark.appendChild(icon("server"));
    const serverCopy = element("div", "community-server-copy");
    serverCopy.append(element("strong", "", preset.name), element("span", "community-server-mode", preset.mode));
    identity.append(serverMark, serverCopy);
    const state = element("span", "community-server-state", "等待查询");
    const body = element("div", "community-server-body");
    const title = element("div", "community-server-title", "状态尚未更新");
    const map = element("div", "community-server-map", "等待查询地图");
    body.append(element("span", "safehouse-field-label", "当前地图"), map, title);
    const footer = element("div", "community-server-footer");
    const address = element("div", "community-server-address");
    const info = element("div", "community-server-info", "");
    address.append(element("code", "", preset.address), info);
    const connect = button("加入服务器", async () => {
      connect.disabled = true;
      try { await ConnectToServer(preset.address); }
      catch (error) { info.textContent = `连接失败：${String(error?.message || error)}`; }
      finally { connect.disabled = false; }
    }, true);
    withIcon(connect, "arrow");
    cardHeading.append(identity, state);
    footer.append(address, connect);
    card.append(cardHeading, body, footer);
    statusViews.set(preset.id, { state, title, map, info });
    cards.appendChild(card);
  }
  view.append(heading, cards);

  const sync = element("section", "community-map-sync");
  const syncHeading = element("div", "safehouse-section-heading");
  const syncMark = element("div", "safehouse-section-mark");
  syncMark.appendChild(icon("sync"));
  const syncCopy = element("div");
  syncCopy.append(element("h3", "", "同步服务器地图"), element("p", "", "先预览变更，再将缺图加入下载队列。"));
  syncHeading.append(syncMark, syncCopy);
  const sourceField = element("div", "safehouse-source-field");
  const sourceRow = element("div", "map-sync-source-row");
  const sourceLabel = element("label", "", "同步来源");
  sourceLabel.htmlFor = "safehouse-sync-source";
  const source = element("input", "form-input community-sync-source");
  source.id = "safehouse-sync-source";
  source.type = "text";
  source.placeholder = "cd.fastbot.top";
  source.autocomplete = "off";
  source.spellcheck = false;
  source.addEventListener("input", () => {
    invalidatePlan();
    preview.disabled = sourceIsDirty();
  });
  const settings = button("保存", () => saveSource());
  sourceRow.append(source, settings);
  sourceField.append(sourceLabel, sourceRow);
  const directoryRow = element("div", "safehouse-directory");
  const directory = element("p", "community-sync-directory");
  directoryRow.append(icon("folder"), directory);
  const note = element("p", "community-sync-note", "旧地图移入 addons/disabled/server-sync-*，可恢复；清空回收站后将永久删除。其他 MOD 不受影响。");
  const toolbar = element("div", "safehouse-sync-toolbar");
  const actions = element("div", "community-sync-actions");
  const secondaryActions = element("div", "community-sync-actions safehouse-secondary-actions");
  const preview = withIcon(button("预览变更", () => previewSync(), true), "sync");
  const apply = button("同步地图", () => applySync());
  apply.classList.add("safehouse-apply");
  apply.disabled = true;
  const restore = withIcon(button("恢复上次清理", () => restoreSync()), "restore");
  const emptyTrash = withIcon(button("清空回收站", () => clearTrash()), "trash");
  emptyTrash.classList.add("safehouse-empty-trash");
  const downloads = button("查看下载任务", () => switchAppPage("downloads"));
  actions.append(preview, apply);
  secondaryActions.append(restore, emptyTrash, downloads);
  toolbar.append(actions, secondaryActions);
  const cleanupLabel = element("label", "community-sync-checkbox");
  const cleanup = element("input");
  cleanup.type = "checkbox";
  cleanup.checked = true;
  cleanupLabel.append(cleanup, document.createTextNode("同时清理服务器已删除的旧地图"), element("span", "safehouse-recoverable", "可恢复"));
  const cleanupOptions = element("div", "safehouse-cleanup-options");
  const cleanupNote = element("div", "safehouse-cleanup-note");
  cleanupNote.append(icon("shield"), note);
  cleanupOptions.append(cleanupLabel, cleanupNote);
  const message = element("p", "community-sync-message", "预览后可展开查看待下载与待清理的地图。");
  message.setAttribute("role", "status");
  message.setAttribute("aria-live", "polite");
  const lists = element("div", "community-sync-lists");
  sync.append(syncHeading, directoryRow, sourceField, toolbar, cleanupOptions, message, lists);
  view.appendChild(sync);
  parent.prepend(view);

  let currentPlan = null;
  let busy = false;
  let statusBusy = false;
  let requestSequence = 0;
  let lastStatusAt = 0;

  function sourceIsDirty() {
    return source.value.trim() !== (getConfig().mapSyncServer || "cd.fastbot.top");
  }

  function setBusy(value) {
    busy = value;
    preview.disabled = value || sourceIsDirty();
    restore.disabled = value;
    emptyTrash.disabled = value;
    settings.disabled = value;
    source.disabled = value;
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

  async function saveSource() {
    if (busy) return;
    const previous = getConfig().mapSyncServer || "cd.fastbot.top";
    invalidatePlan();
    setBusy(true);
    try {
      const server = normalizeSyncSource(source.value);
      await saveConfig({ mapSyncServer: server });
      source.value = server;
      source.dataset.source = server;
      message.textContent = "同步来源已保存，请重新预览。";
      message.dataset.state = "success";
      lists.replaceChildren();
    } catch (error) {
      try { await saveConfig({ mapSyncServer: previous }); } catch { /* Keep the original error. */ }
      fail(error);
    } finally { setBusy(false); }
  }

  async function updateContext() {
    const configured = getConfig().mapSyncServer || "cd.fastbot.top";
    if (source.dataset.source !== configured) {
      source.dataset.source = configured;
      source.value = configured;
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
        if (available) {
          output.state.replaceChildren(element("span", "", "在线"), element("strong", "community-server-count", `${Number(server.players) || 0}/${Number(server.max_players) || 0}`));
        } else {
          output.state.textContent = "暂不可用";
        }
        output.state.dataset.state = available ? "online" : "offline";
        output.title.textContent = available ? (server.server_name || preset.mode) : "服务器离线或暂时无法查询";
        const chapter = server?.chapter && server?.chapter_count ? ` ${server.chapter}/${server.chapter_count}` : "";
        output.map.textContent = available ? `${server.map_title || server.map || "未知地图"}${chapter}` : "暂无地图信息";
        output.info.textContent = available ? `查询延迟 ${Number(server.ping_ms) || 0} ms` : "可尝试直接连接";
      }
      lastStatusAt = Date.now();
    } catch (error) {
      for (const preset of COMMUNITY_SERVER_PRESETS) {
        const output = statusViews.get(preset.id);
        output.state.textContent = "状态查询失败";
        output.state.dataset.state = "error";
        output.title.textContent = preset.mode;
        output.map.textContent = "暂无地图信息";
        output.info.textContent = String(error?.message || error);
      }
    } finally {
      statusBusy = false;
      refresh.disabled = false;
    }
  }

  async function previewSync() {
    if (busy) return;
    if (sourceIsDirty()) {
      message.textContent = "请先保存同步来源，再预览地图。";
      return;
    }
    invalidatePlan();
    setBusy(true);
    message.textContent = "正在查询服务器清单、删除记录并比对本机地图…";
    message.dataset.state = "";
    lists.replaceChildren();
    try {
      const plan = await PreviewServerMapSync();
      if (!plan?.id) throw new Error("未获得有效的同步预览");
      currentPlan = plan;
      source.value = plan.server;
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

  async function clearTrash() {
    if (busy) return;
    setBusy(true);
    try {
      const trash = await PreviewServerMapSyncTrash();
      if (!trash?.id) throw new Error("未获得有效的回收站预览");
      const skipped = trash.skipped ? ` 已保留 ${trash.skipped} 个无法确认内容的目录。` : "";
      if (!trash.batches) {
        message.textContent = `地图同步回收站为空。${skipped}`;
        message.dataset.state = "";
        return;
      }
      showConfirmModal(
        "清空地图同步回收站？",
        `将永久删除 ${trash.maps || 0} 张地图及关联文件（共 ${trash.files || 0} 个文件，约 ${formatSyncBytes(trash.bytes)}）。范围：${trash.directory}/disabled/server-sync-*。清空后无法恢复，其他禁用 MOD 不受影响。${skipped}`,
        async () => {
          if (busy) return false;
          setBusy(true);
          try {
            const result = await ClearServerMapSyncTrash(trash.id);
            invalidatePlan();
            lists.replaceChildren();
            const errors = Array.isArray(result.errors) ? result.errors : [];
            message.textContent = `已清理 ${result.files || 0} 个文件，释放约 ${formatSyncBytes(result.bytes)}。${skipped}${errors.length ? ` 部分文件未清理：${errors.join("；")}` : ""}`;
            message.dataset.state = errors.length ? "error" : "success";
          } catch (error) {
            invalidatePlan();
            fail(error);
          } finally { setBusy(false); }
        },
      );
    } catch (error) { fail(error); }
    finally { setBusy(false); }
  }

  const pageChanged = (event) => {
    if (event.detail?.page !== "safehouse") return;
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

export function mountSafehouseView(parent, services) {
  if (!parent) return;
  const existing = mountedViews.get(parent);
  if (existing?.view.isConnected) {
    existing.updateContext();
    return existing;
  }
  const result = createCommunityView(parent, services);
  mountedViews.set(parent, result);
  return result;
}
