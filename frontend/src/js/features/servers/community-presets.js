export const COMMUNITY_SERVER_PRESETS = [
  { id: "cd", name: "成都 · 战役服", address: "cd.fastbot.top", mode: "战役 · 普通难度" },
  { id: "gd", name: "广东 · 对抗练习服", address: "gd.fastbot.top", mode: "ZoneMod · 4v4 + 观众" },
];

export function mergeCommunityPresets(recentServers = []) {
  const result = [];
  const seen = new Set();
  for (const server of [...COMMUNITY_SERVER_PRESETS, ...recentServers]) {
    const key = String(server.address || "").trim().toLowerCase().replace(/:27015$/, "");
    if (!key || seen.has(key)) continue;
    seen.add(key);
    result.push(server);
  }
  return result;
}

export function normalizeSyncSource(value) {
  const text = String(value || "").trim() || "cd.fastbot.top";
  let address;
  try {
    address = new URL(text.includes("://") ? text : `https://${text}`);
  } catch {
    throw new Error("请输入有效的服务器域名或 HTTPS 地址");
  }
  if (address.protocol !== "https:" || !address.hostname || address.username || address.password || address.search || address.hash || (address.pathname && address.pathname !== "/")) {
    throw new Error("请输入服务器域名或 HTTPS 地址，不要包含路径、密码或参数");
  }
  return address.host.toLowerCase();
}

export function formatSyncBytes(bytes) {
  const count = Number(bytes) || 0;
  if (count < 1024) return `${count} B`;
  if (count < 1024 ** 2) return `${(count / 1024).toFixed(1)} KB`;
  if (count < 1024 ** 3) return `${(count / 1024 ** 2).toFixed(1)} MB`;
  return `${(count / 1024 ** 3).toFixed(2)} GB`;
}
