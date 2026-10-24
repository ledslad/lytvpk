import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const source = await readFile(new URL("../src/js/features/safehouse/presets.js", import.meta.url), "utf8");
const { COMMUNITY_SERVER_PRESETS, normalizeSyncSource } = await import(`data:text/javascript;base64,${Buffer.from(source).toString("base64")}`);

test("safehouse has the two community servers without modifying saved favorites", () => {
  assert.deepEqual(COMMUNITY_SERVER_PRESETS.map(({ id, address }) => [id, address]), [
    ["cd", "cd.fastbot.top"], ["gd", "gd.fastbot.top"],
  ]);
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
