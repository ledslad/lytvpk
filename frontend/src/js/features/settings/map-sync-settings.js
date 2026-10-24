import { normalizeSyncSource } from "../servers/community-presets.js";

export function mountMapSyncSettings({ getConfig, saveConfig }) {
  const panel = document.getElementById("settings-panel-network");
  if (!panel || panel.querySelector(".map-sync-settings")) return;
  const card = document.createElement("section");
  card.className = "setting-card map-sync-settings";
  const title = document.createElement("div");
  title.className = "setting-card-title";
  title.textContent = "服务器地图同步";
  const label = document.createElement("label");
  label.textContent = "默认同步服务器";
  label.htmlFor = "map-sync-server-input";
  const row = document.createElement("div");
  row.className = "map-sync-source-row";
  const input = document.createElement("input");
  input.id = "map-sync-server-input";
  input.type = "text";
  input.className = "form-input";
  input.placeholder = "cd.fastbot.top";
  input.value = getConfig().mapSyncServer || "cd.fastbot.top";
  input.autocomplete = "off";
  input.spellcheck = false;
  const save = document.createElement("button");
  save.type = "button";
  save.className = "btn btn-primary";
  save.textContent = "保存服务器";
  const hint = document.createElement("p");
  hint.className = "community-sync-note";
  hint.textContent = "填写支持公开地图同步接口的域名或 HTTPS 地址。默认使用 cd.fastbot.top，无需 token；在服务器页面预览并执行同步。";
  const status = document.createElement("p");
  status.className = "community-sync-message";
  status.setAttribute("role", "status");
  save.addEventListener("click", async () => {
    const previous = getConfig().mapSyncServer || "cd.fastbot.top";
    save.disabled = true;
    try {
      const server = normalizeSyncSource(input.value);
      await saveConfig({ mapSyncServer: server });
      input.value = server;
      status.textContent = `已保存同步服务器：${server}`;
      status.dataset.state = "success";
    } catch (error) {
      // saveConfig updates its in-memory cache before disk persistence.
      // Restore the previous setting when the write failed.
      try { await saveConfig({ mapSyncServer: previous }); } catch { /* Show the original error. */ }
      status.textContent = String(error?.message || error);
      status.dataset.state = "error";
    } finally { save.disabled = false; }
  });
  row.append(input, save);
  card.append(title, label, row, hint, status);
  panel.appendChild(card);
}
