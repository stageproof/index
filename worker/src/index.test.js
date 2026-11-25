import assert from "node:assert/strict";
import test from "node:test";

import worker, { NOT_INDEXED, resetForTests } from "./index.js";

const registry = "https://charts.example/fixtures/";
const indexed = "<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>\n";
const pages = "https://stageproof.github.io/index";
const svgURL = pages + "/badge/fixtures/web/0.1.0/0.2.0.svg";

function query(extra) {
  const url = new URL("https://worker.test/badge.svg");
  url.searchParams.set("chart", "web");
  url.searchParams.set("from", "0.1.0");
  url.searchParams.set("registry", registry);
  url.searchParams.set("to", "0.2.0");
  if (extra) {
    for (const [key, value] of Object.entries(extra)) {
      url.searchParams.set(key, value);
    }
  }
  return url;
}

function installFetch(routes) {
  const calls = [];
  global.fetch = async (url) => {
    calls.push(String(url));
    const hit = routes[String(url)];
    if (!hit) {
      return new Response("missing", { status: 404 });
    }
    return new Response(hit.body, { status: hit.status || 200 });
  };
  return calls;
}

test.beforeEach(() => {
  resetForTests();
});

test("an indexed pair returns the stored SVG", async () => {
  const calls = installFetch({
    [pages + "/badge/registries.json"]: {
      body: JSON.stringify({ [registry.replace(/\/+$/, "")]: { web: "fixtures" } }),
    },
    [svgURL]: { body: indexed },
  });
  const res = await worker.fetch(new Request(query()));
  assert.equal(res.status, 200);
  assert.equal(await res.text(), indexed);
  assert.equal(res.headers.get("content-type"), "image/svg+xml; charset=utf-8");
  assert.equal(res.headers.get("x-content-type-options"), "nosniff");
  assert.equal(res.headers.get("content-security-policy"), "default-src 'none'; style-src 'unsafe-inline'");
  assert.equal(res.headers.get("cache-control"), "public, max-age=300");
  const again = await worker.fetch(new Request(query()));
  assert.equal(await again.text(), indexed);
  assert.equal(calls.filter((url) => url === svgURL).length, 1);
});

test("an unknown chart is the not-indexed image", async () => {
  installFetch({
    [pages + "/badge/registries.json"]: { body: JSON.stringify({}) },
  });
  const res = await worker.fetch(new Request(query()));
  assert.equal(res.status, 200);
  assert.equal(await res.text(), NOT_INDEXED);
});

test("a missing file is the not-indexed image", async () => {
  const calls = installFetch({
    [pages + "/badge/registries.json"]: {
      body: JSON.stringify({ [registry.replace(/\/+$/, "")]: { web: "fixtures" } }),
    },
    [svgURL]: { status: 404, body: "nope" },
  });
  const res = await worker.fetch(new Request(query()));
  assert.equal(await res.text(), NOT_INDEXED);
  const again = await worker.fetch(new Request(query()));
  assert.equal(await again.text(), NOT_INDEXED);
  assert.equal(calls.filter((url) => url === svgURL).length, 2);
});

test("a body that is not SVG is the not-indexed image", async () => {
  installFetch({
    [pages + "/badge/registries.json"]: {
      body: JSON.stringify({ [registry.replace(/\/+$/, "")]: { web: "fixtures" } }),
    },
    [svgURL]: { body: "not an image" },
  });
  const res = await worker.fetch(new Request(query()));
  assert.equal(await res.text(), NOT_INDEXED);
});

test("a bad path segment is the not-indexed image", async () => {
  const calls = installFetch({});
  const res = await worker.fetch(new Request(query({ chart: "../web" })));
  assert.equal(await res.text(), NOT_INDEXED);
  assert.equal(calls.length, 0);
});

test("a non-GET method is rejected", async () => {
  const calls = installFetch({});
  const res = await worker.fetch(new Request(query(), { method: "POST" }));
  assert.equal(res.status, 405);
  assert.equal(res.headers.get("x-content-type-options"), "nosniff");
  assert.equal(calls.length, 0);
});

test("HEAD returns the image headers and no body", async () => {
  installFetch({
    [pages + "/badge/registries.json"]: {
      body: JSON.stringify({ [registry.replace(/\/+$/, "")]: { web: "fixtures" } }),
    },
    [svgURL]: { body: indexed },
  });
  const res = await worker.fetch(new Request(query(), { method: "HEAD" }));
  assert.equal(res.status, 200);
  assert.equal(await res.text(), "");
  assert.equal(res.headers.get("content-type"), "image/svg+xml; charset=utf-8");
});
