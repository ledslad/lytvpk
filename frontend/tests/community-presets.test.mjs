import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const source = await readFile(new URL("../src/js/features/servers/community-presets.js", import.meta.url), "utf8");
const { mergeCommunityPresets, normalizeSyncSource } = await import(`data:text/javascript;base64,${Buffer.from(source).toString("base64")}`);

test("presets preserve recent servers without duplicating their default port", () => {
  const servers = mergeCommunityPresets([
    { name: "my cd name", address: "CD.FASTBOT.TOP:27015" },
    { name: "another server", address: "example.org:27016" },
    { name: "gd recent", address: "gd.fastbot.top" },
  ]);
  assert.deepEqual(servers.map((server) => server.address), ["cd.fastbot.top", "gd.fastbot.top", "example.org:27016"]);
});

test("sync source defaults to cd and accepts HTTPS hosts", () => {
  assert.equal(normalizeSyncSource(""), "cd.fastbot.top");
  assert.equal(normalizeSyncSource(" https://CD.fastbot.top/ "), "cd.fastbot.top");
  assert.equal(normalizeSyncSource("https://example.org:8443"), "example.org:8443");
});

test("sync source rejects credentials, query strings and non-HTTPS transports", () => {
  for (const value of ["http://cd.fastbot.top", "https://user:token@example.org", "https://example.org/api/maps", "https://example.org?token=secret", "https://example.org/#token"]) {
    assert.throws(() => normalizeSyncSource(value));
  }
});
