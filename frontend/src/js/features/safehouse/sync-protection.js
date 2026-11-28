export function createSyncProtectionPanel(services, onChange) {
  const panel = document.createElement("section");
  panel.className = "safehouse-sync-protection";

  const toggle = document.createElement("button");
  toggle.type = "button";
  toggle.className = "btn btn-secondary";
  toggle.textContent = "本机保护白名单";
  toggle.setAttribute("aria-expanded", "false");
  toggle.setAttribute("aria-controls", "safehouse-sync-protected-files");

  const body = document.createElement("div");
  body.id = "safehouse-sync-protected-files";
  body.hidden = true;

  const help = document.createElement("p");
  help.className = "community-sync-note";
  help.textContent = "填写 VPK 文件名。所有同步来源都会跳过同名下载、替换和旧图清理。";

  const form = document.createElement("form");
  form.className = "map-sync-source-row";
  const filename = document.createElement("input");
  filename.className = "form-input";
  filename.placeholder = "例如 my-mod.vpk";
  filename.required = true;
  filename.spellcheck = false;
  filename.setAttribute("aria-label", "要保护的 VPK 文件名");
  const add = document.createElement("button");
  add.className = "btn btn-primary";
  add.type = "submit";
  add.textContent = "添加保护";
  form.append(filename, add);

  const status = document.createElement("p");
  status.className = "community-sync-note";
  status.setAttribute("role", "status");
  const list = document.createElement("ul");
  body.append(help, form, status, list);
  panel.append(toggle, body);

  let pending = false;
  function setPending(value) {
    pending = value;
    filename.disabled = value;
    add.disabled = value;
    for (const row of list.children) row.children[1].disabled = value;
  }

  function render(files) {
    list.replaceChildren();
    status.textContent = files.length ? `已保护 ${files.length} 个文件` : "暂未添加保护文件";
    for (const file of files) {
      const row = document.createElement("li");
      const name = document.createElement("span");
      name.textContent = file;
      const remove = document.createElement("button");
      remove.type = "button";
      remove.className = "btn btn-secondary";
      remove.textContent = "取消保护";
      remove.setAttribute("aria-label", `取消保护 ${file}`);
      remove.addEventListener("click", () => save(file, false));
      row.append(name, remove);
      list.append(row);
    }
  }

  async function save(name, protectedFile) {
    if (pending) return;
    setPending(true);
    try {
      render(await services.SetServerMapSyncFileProtected(name, protectedFile));
      if (protectedFile) filename.value = "";
      onChange();
    } catch (error) {
      status.textContent = String(error?.message || error);
    } finally {
      setPending(false);
    }
  }

  form.addEventListener("submit", (event) => {
    event.preventDefault();
    return save(filename.value.trim(), true);
  });
  toggle.addEventListener("click", async () => {
    body.hidden = !body.hidden;
    toggle.setAttribute("aria-expanded", String(!body.hidden));
    if (body.hidden || pending) return;
    setPending(true);
    try {
      render(await services.GetServerMapSyncProtectedFiles());
    } catch (error) {
      status.textContent = String(error?.message || error);
    } finally {
      setPending(false);
    }
  });

  return panel;
}
