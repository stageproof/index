// Answers /badge.svg for a Renovate pull request.
// Indexed SVG files stay on GitHub Pages. A missing pair is HTTP 200, not a 404.
// The not-indexed document must match site.UnknownSVG. TestBadgeURLSchemeFrozen checks it.

const PAGES = "https://stageproof.github.io/index";

const NOT_INDEXED = `<svg xmlns="http://www.w3.org/2000/svg" width="171" height="20" role="img" aria-label="stageproof: not indexed">
<title>stageproof: not indexed</title>
<rect width="82" height="20" fill="#555"/>
<rect x="82" width="89" height="20" fill="#6c757d"/>
<g fill="#fff" text-anchor="middle" font-family="Verdana,DejaVu Sans,sans-serif" font-size="11">
<text x="41" y="14">stageproof</text>
<text x="126" y="14">not indexed</text>
</g>
</svg>
`;

let cache = { at: 0, map: null };

export default {
  async fetch(request) {
    const url = new URL(request.url);
    if (url.pathname !== "/badge.svg") {
      return text("not found", 404);
    }
    try {
      return svg(await badge(url));
    } catch (_err) {
      return svg(NOT_INDEXED);
    }
  },
};

function segment(value) {
  return typeof value === "string" && value !== "" && value !== "." && value !== ".." && !value.includes("..") && !/[\\/?#]/.test(value);
}

function norm(value) {
  return value.trim().replace(/\/+$/, "");
}

async function loadMap() {
  const now = Date.now();
  if (cache.map && now - cache.at < 300000) {
    return cache.map;
  }
  const res = await fetch(PAGES + "/badge/registries.json");
  if (!res.ok) {
    return {};
  }
  const map = await res.json();
  cache = { at: now, map };
  return map;
}

async function badge(url) {
  const registry = url.searchParams.get("registry") || "";
  const chart = url.searchParams.get("chart") || "";
  const from = url.searchParams.get("from") || "";
  const to = url.searchParams.get("to") || "";
  if (!segment(chart) || !segment(from) || !segment(to) || norm(registry) === "") {
    return NOT_INDEXED;
  }
  const map = await loadMap();
  const charts = map[norm(registry)];
  const repo = charts && charts[chart];
  if (!segment(repo)) {
    return NOT_INDEXED;
  }
  const file = PAGES + "/badge/" + encodeURIComponent(repo) + "/" + encodeURIComponent(chart) + "/" + encodeURIComponent(from) + "/" + encodeURIComponent(to) + ".svg";
  const res = await fetch(file);
  if (!res.ok) {
    return NOT_INDEXED;
  }
  const body = await res.text();
  if (!body.startsWith("<svg")) {
    return NOT_INDEXED;
  }
  return body;
}

function svg(body) {
  return new Response(body, {
    status: 200,
    headers: {
      "content-type": "image/svg+xml; charset=utf-8",
      "cache-control": "public, max-age=300",
    },
  });
}

function text(body, status) {
  return new Response(body, {
    status,
    headers: { "content-type": "text/plain; charset=utf-8" },
  });
}
