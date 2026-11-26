// Browser-only values check. The paste is read in this page and is not sent.
// Stored reports come from chart defaults. Key lists come from findings in that JSON.

(function (root, factory) {
  var api = factory();
  if (typeof module === "object" && module.exports) {
    module.exports = api;
  }
  if (typeof document !== "undefined") {
    api.boot(document);
  }
  root.stageproofValues = api;
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  var RULES = {
    "helm.values.silently_dropped": "dropped",
    "helm.values.renamed": "renamed",
    "helm.values.default_changed": "changed",
    "helm.values.new_required_unset": "required"
  };

  function formatPath(segs) {
    var out = "";
    for (var i = 0; i < segs.length; i++) {
      var s = segs[i];
      if (/[./:]/.test(s)) {
        out += "[" + JSON.stringify(s) + "]";
      } else {
        if (i > 0) {
          out += ".";
        }
        out += s;
      }
    }
    return out;
  }

  function stripComment(line) {
    var quote = "";
    for (var i = 0; i < line.length; i++) {
      var c = line[i];
      if (quote) {
        if (quote === "'" && c === "'" && line[i + 1] === "'") {
          i++;
          continue;
        }
        if (c === quote) {
          if (quote === '"' && line[i - 1] === "\\") {
            continue;
          }
          quote = "";
        }
        continue;
      }
      if (c === '"' || c === "'") {
        quote = c;
        continue;
      }
      if (c === "#" && (i === 0 || line[i - 1] === " " || line[i - 1] === "\t")) {
        return line.slice(0, i).replace(/[ \t]+$/, "");
      }
    }
    return line.replace(/[ \t]+$/, "");
  }

  function unquote(raw) {
    if (raw.length >= 2 && raw[0] === '"' && raw[raw.length - 1] === '"') {
      try {
        return JSON.parse(raw);
      } catch (_err) {
        return raw.slice(1, -1);
      }
    }
    if (raw.length >= 2 && raw[0] === "'" && raw[raw.length - 1] === "'") {
      return raw.slice(1, -1).replace(/''/g, "'");
    }
    return raw;
  }

  function splitKey(text) {
    var src = text.trim();
    if (src.startsWith("- ")) {
      src = src.slice(2).trim();
    }
    if (src === "" || src === "-") {
      return null;
    }
    var key;
    var rest;
    if (src[0] === '"' || src[0] === "'") {
      var q = src[0];
      var end = 1;
      while (end < src.length) {
        if (q === "'" && src[end] === "'" && src[end + 1] === "'") {
          end += 2;
          continue;
        }
        if (q === '"' && src[end] === "\\") {
          end += 2;
          continue;
        }
        if (src[end] === q) {
          break;
        }
        end++;
      }
      key = unquote(src.slice(0, end + 1));
      rest = src.slice(end + 1).trim();
      if (rest[0] !== ":") {
        return null;
      }
      rest = rest.slice(1).trim();
    } else {
      var colon = -1;
      for (var c = 0; c < src.length; c++) {
        if (src[c] === ":" && (c + 1 === src.length || src[c + 1] === " " || src[c + 1] === "\t")) {
          colon = c;
          break;
        }
      }
      if (colon <= 0) {
        return null;
      }
      key = src.slice(0, colon).trim();
      rest = src.slice(colon + 1).trim();
    }
    var anchor = "";
    var alias = "";
    var aliases = [];
    if (rest.startsWith("&")) {
      var parts = rest.split(/\s+/);
      anchor = parts[0].slice(1);
      rest = parts.slice(1).join(" ");
    }
    if (rest.startsWith("*")) {
      alias = rest.slice(1).split(/\s+/)[0];
      rest = "";
    } else if (key === "<<" && rest.startsWith("[") && rest.endsWith("]")) {
      var items = rest.slice(1, -1).split(",");
      for (var a = 0; a < items.length; a++) {
        var tok = items[a].trim();
        if (tok.startsWith("*")) {
          aliases.push(tok.slice(1));
        }
      }
      rest = "";
    }
    var flow = "";
    if (rest.startsWith("{") && rest.endsWith("}")) {
      flow = rest;
      rest = "";
    }
    var blockScalar = /^[|>][0-9+-]*$/.test(rest);
    return {
      key: key,
      anchor: anchor,
      alias: alias,
      aliases: aliases,
      flow: flow,
      blockScalar: blockScalar,
      nested: !blockScalar && (rest === "" || rest === "null" || rest === "~")
    };
  }

  function linesOf(text) {
    var rows = String(text).replace(/^\uFEFF/, "").replace(/\r\n/g, "\n").split("\n");
    var out = [];
    for (var i = 0; i < rows.length; i++) {
      var raw = stripComment(rows[i]);
      var trimmed = raw.trim();
      if (trimmed === "" || trimmed === "---" || trimmed === "...") {
        continue;
      }
      var indent = 0;
      while (raw[indent] === " ") {
        indent++;
      }
      while (raw[indent] === "\t") {
        indent += 2;
      }
      out.push({ indent: indent, text: trimmed });
    }
    return out;
  }

  function clone(node) {
    if (!node || node.kind === "scalar") {
      return { kind: "scalar" };
    }
    if (node.kind === "seq") {
      return { kind: "seq", items: node.items.map(clone) };
    }
    return {
      kind: "map",
      entries: node.entries.map(function (e) {
        return { key: e.key, value: clone(e.value) };
      })
    };
  }

  function put(map, key, value) {
    for (var i = 0; i < map.entries.length; i++) {
      if (map.entries[i].key === key) {
        map.entries[i].value = value;
        return;
      }
    }
    map.entries.push({ key: key, value: value });
  }

  function merge(map, other) {
    if (!other || other.kind !== "map") {
      return;
    }
    for (var i = 0; i < other.entries.length; i++) {
      var e = other.entries[i];
      var exists = false;
      for (var j = 0; j < map.entries.length; j++) {
        if (map.entries[j].key === e.key) {
          exists = true;
          break;
        }
      }
      if (!exists && e.key !== "<<") {
        map.entries.push({ key: e.key, value: clone(e.value) });
      }
    }
  }

  function parseFlow(text, anchors) {
    var body = text.trim();
    if (body[0] !== "{" || body[body.length - 1] !== "}") {
      return { kind: "scalar" };
    }
    body = body.slice(1, -1);
    var node = { kind: "map", entries: [] };
    var depth = 0;
    var quote = "";
    var start = 0;
    var parts = [];
    for (var i = 0; i < body.length; i++) {
      var c = body[i];
      if (quote) {
        if (c === quote && body[i - 1] !== "\\") {
          quote = "";
        }
        continue;
      }
      if (c === '"' || c === "'") {
        quote = c;
        continue;
      }
      if (c === "{") {
        depth++;
      } else if (c === "}") {
        depth--;
      } else if (c === "," && depth === 0) {
        parts.push(body.slice(start, i));
        start = i + 1;
      }
    }
    parts.push(body.slice(start));
    for (var p = 0; p < parts.length; p++) {
      var piece = parts[p].trim();
      if (piece === "") {
        continue;
      }
      var item = splitKey(piece);
      if (!item) {
        continue;
      }
      var value = { kind: "scalar" };
      if (item.flow) {
        value = parseFlow(item.flow, anchors);
      } else if (item.alias && anchors[item.alias]) {
        value = clone(anchors[item.alias]);
      }
      if (item.key === "<<") {
        merge(node, value);
      } else {
        put(node, item.key, value);
      }
    }
    return node;
  }

  function isSeqLine(text) {
    return text === "-" || text.startsWith("- ");
  }

  function parseSeq(lines, i, indent, anchors) {
    var items = [];
    while (i < lines.length && lines[i].indent === indent && isSeqLine(lines[i].text)) {
      var rest = lines[i].text === "-" ? "" : lines[i].text.slice(2).trim();
      var item = { kind: "scalar" };
      i++;
      if (rest === "") {
        if (i < lines.length && lines[i].indent > indent && !isSeqLine(lines[i].text)) {
          var nested = parseMap(lines, i, lines[i].indent, anchors);
          item = nested.node;
          i = nested.next;
        }
      } else if (splitKey(rest)) {
        var synthetic = [{ indent: indent + 2, text: rest }];
        var j = i;
        while (j < lines.length && lines[j].indent > indent && !isSeqLine(lines[j].text)) {
          synthetic.push(lines[j]);
          j++;
        }
        var parsed = parseMap(synthetic, 0, indent + 2, anchors);
        item = parsed.node;
        i = j;
      }
      items.push(item);
    }
    return { node: { kind: "seq", items: items }, next: i };
  }

  function parseMap(lines, i, indent, anchors) {
    var node = { kind: "map", entries: [] };
    while (i < lines.length) {
      if (lines[i].indent !== indent || isSeqLine(lines[i].text)) {
        break;
      }
      var item = splitKey(lines[i].text);
      i++;
      if (!item) {
        continue;
      }
      if (item.blockScalar) {
        while (i < lines.length && lines[i].indent > indent) {
          i++;
        }
        put(node, item.key, { kind: "scalar" });
        continue;
      }
      var value = { kind: "scalar" };
      if (item.aliases.length > 0) {
        for (var a = 0; a < item.aliases.length; a++) {
          if (anchors[item.aliases[a]]) {
            merge(node, clone(anchors[item.aliases[a]]));
          }
        }
        continue;
      }
      if (item.alias) {
        value = anchors[item.alias] ? clone(anchors[item.alias]) : { kind: "scalar" };
      } else if (item.flow) {
        value = parseFlow(item.flow, anchors);
      } else if (item.nested && i < lines.length && lines[i].indent > indent) {
        if (isSeqLine(lines[i].text)) {
          var seq = parseSeq(lines, i, lines[i].indent, anchors);
          value = seq.node;
          i = seq.next;
        } else {
          var child = parseMap(lines, i, lines[i].indent, anchors);
          value = child.node;
          i = child.next;
        }
      }
      if (item.anchor) {
        anchors[item.anchor] = value;
      }
      if (item.key === "<<") {
        merge(node, value);
        continue;
      }
      put(node, item.key, value);
    }
    return { node: node, next: i };
  }

  function walk(node, prefix, out) {
    if (!node || node.kind !== "map") {
      return;
    }
    for (var i = 0; i < node.entries.length; i++) {
      var e = node.entries[i];
      if (e.key === "<<") {
        continue;
      }
      var segs = prefix.concat([e.key]);
      out.push(formatPath(segs));
      if (e.value && e.value.kind === "seq") {
        for (var n = 0; n < e.value.items.length; n++) {
          var item = e.value.items[n];
          if (item && item.kind === "map") {
            var indexed = prefix.concat([e.key + "[" + n + "]"]);
            walk(item, indexed, out);
            walk(item, segs, out);
          }
        }
      } else {
        walk(e.value, segs, out);
      }
    }
  }

  function keyPaths(text) {
    var lines = linesOf(text);
    var anchors = {};
    var parsed = parseMap(lines, 0, lines.length ? lines[0].indent : 0, anchors);
    var found = [];
    walk(parsed.node, [], found);
    var seen = {};
    var out = [];
    for (var i = 0; i < found.length; i++) {
      if (!seen[found[i]]) {
        seen[found[i]] = true;
        out.push(found[i]);
      }
    }
    out.sort();
    return out;
  }

  function hits(paths, resource) {
    for (var i = 0; i < paths.length; i++) {
      var p = paths[i];
      if (p === resource || p.indexOf(resource + ".") === 0 || p.indexOf(resource + "[") === 0) {
        return true;
      }
    }
    return false;
  }

  function intersect(paths, report) {
    var groups = { dropped: [], renamed: [], changed: [], required: [], blockers: [] };
    var byKey = {};
    var blockerByKey = {};
    var results = (report && report.k8s_results) || {};
    var minors = Object.keys(results).sort();
    for (var m = 0; m < minors.length; m++) {
      var minor = minors[m];
      var findings = (results[minor] && results[minor].findings) || [];
      for (var i = 0; i < findings.length; i++) {
        var f = findings[i];
        if (!f || f.ignored) {
          continue;
        }
        var bucket = RULES[f.rule_id];
        if (bucket && f.resource) {
          var key = bucket + "\x00" + f.resource;
          if (!byKey[key]) {
            byKey[key] = {
              minor: minor,
              minors: [minor],
              resource: f.resource,
              message: f.message || "",
              detail: f.detail || "",
              rule: f.rule_id,
              severity: f.severity || "",
              hit: hits(paths, f.resource)
            };
            groups[bucket].push(byKey[key]);
          } else if (byKey[key].minors.indexOf(minor) < 0) {
            byKey[key].minors.push(minor);
          }
        } else if (f.severity === "blocker") {
          var bkey = (f.rule_id || "") + "\x00" + (f.resource || "") + "\x00" + (f.message || "");
          if (!blockerByKey[bkey]) {
            blockerByKey[bkey] = {
              rule: f.rule_id || "",
              resource: f.resource || "",
              message: f.message || "",
              detail: f.detail || "",
              minors: [minor]
            };
            groups.blockers.push(blockerByKey[bkey]);
          } else if (blockerByKey[bkey].minors.indexOf(minor) < 0) {
            blockerByKey[bkey].minors.push(minor);
          }
        }
      }
    }
    return groups;
  }

  function overallVerdict(report) {
    var results = (report && report.k8s_results) || {};
    var minors = Object.keys(results).sort();
    var sawNotChecked = false;
    for (var i = 0; i < minors.length; i++) {
      var v = (results[minors[i]] && results[minors[i]].verdict) || "";
      if (v === "BLOCKED") {
        return { verdict: "Blocked", cls: "bad", minors: minors };
      }
      if (v === "NOT CHECKED") {
        sawNotChecked = true;
      }
    }
    if (sawNotChecked) {
      return { verdict: "Not checked", cls: "neutral", minors: minors };
    }
    return { verdict: "No blocking finding", cls: "ok", minors: minors };
  }

  function uniqueFroms(pairs) {
    var seen = {};
    var out = [];
    for (var i = 0; i < pairs.length; i++) {
      var f = pairs[i].from;
      if (!seen[f]) {
        seen[f] = true;
        out.push(f);
      }
    }
    return out;
  }

  function targetsForFrom(pairs, from) {
    var out = [];
    for (var i = 0; i < pairs.length; i++) {
      if (pairs[i].from === from) {
        out.push(pairs[i]);
      }
    }
    return out;
  }

  function pickDefaultPair(doc, wantFrom, wantTo) {
    var pairs = (doc && doc.pairs) || [];
    if (!pairs.length) {
      return { from: "", to: "" };
    }
    if (wantFrom && wantTo) {
      for (var i = 0; i < pairs.length; i++) {
        if (pairs[i].from === wantFrom && pairs[i].to === wantTo) {
          return { from: wantFrom, to: wantTo };
        }
      }
    }
    if (doc.default_from && doc.default_to) {
      return { from: doc.default_from, to: doc.default_to };
    }
    for (var j = 0; j < pairs.length; j++) {
      if (pairs[j].adjacent) {
        return { from: pairs[j].from, to: pairs[j].to };
      }
    }
    return { from: pairs[0].from, to: pairs[0].to };
  }

  function segment(s) {
    return typeof s === "string" && s !== "" && s !== "." && s !== ".." && s.indexOf("..") < 0 && !/[\/\\?#]/.test(s);
  }

  function encSeg(s) {
    return encodeURIComponent(s).replace(/%2B/g, "+");
  }

  function chartURL(root, repo, chart) {
    if (!segment(repo) || !segment(chart)) {
      throw new Error("That chart name cannot be used in a path.");
    }
    var path = root + "/api/v1/chart/" + encSeg(repo) + "/" + encSeg(chart) + ".json";
    if (path.indexOf(root + "/api/v1/") !== 0) {
      throw new Error("Refusing a chart URL outside this site.");
    }
    return path;
  }

  function reportURL(root, repo, chart, from, to) {
    var parts = [repo, chart, from, to];
    for (var i = 0; i < parts.length; i++) {
      if (!segment(parts[i])) {
        throw new Error("That name cannot be used in a path.");
      }
    }
    var path = root + "/api/v1/upgrade/" + parts.map(encSeg).join("/") + ".json";
    if (path.indexOf(root + "/api/v1/") !== 0) {
      throw new Error("Refusing a report URL outside this site.");
    }
    return path;
  }

  function upgradePageURL(root, repo, chart, from, to) {
    return root + "/upgrade/" + [repo, chart, from, to].map(encSeg).join("/") + "/";
  }

  function cliWithValues(reproduceCommand) {
    if (!reproduceCommand) {
      return "";
    }
    if (reproduceCommand.indexOf(" -f ") >= 0 || reproduceCommand.indexOf(" --values ") >= 0) {
      return reproduceCommand;
    }
    return reproduceCommand + " -f values.yaml";
  }

  function formatMinors(minors) {
    if (!minors || !minors.length) {
      return "";
    }
    if (minors.length === 1) {
      return "K8s " + minors[0];
    }
    return "K8s " + minors[0] + " - " + minors[minors.length - 1];
  }

  function el(doc, tag, text, cls) {
    var node = doc.createElement(tag);
    if (text !== undefined && text !== null) {
      node.textContent = text;
    }
    if (cls) {
      node.className = cls;
    }
    return node;
  }

  function badge(doc, text, kind) {
    return el(doc, "span", text, "badge badge-" + kind);
  }

  function renderResults(doc, host, ctx) {
    host.textContent = "";
    var groups = ctx.groups;
    var hasPaste = ctx.paths.length > 0;
    var verdict = overallVerdict(ctx.report);

    var attention = 0;
    var covered = 0;
    for (var a = 0; a < groups.dropped.length; a++) {
      if (groups.dropped[a].hit) { attention++; }
    }
    for (var b = 0; b < groups.renamed.length; b++) {
      if (groups.renamed[b].hit) { attention++; }
    }
    for (var c = 0; c < groups.required.length; c++) {
      if (groups.required[c].hit) { covered++; } else { attention++; }
    }
    for (var d = 0; d < groups.changed.length; d++) {
      if (groups.changed[d].hit) { covered++; } else if (hasPaste) { attention++; }
    }

    var totalValKeys = groups.dropped.length + groups.renamed.length + groups.required.length + groups.changed.length;

    var grid = el(doc, "div", null, "summary-grid");

    var card1 = el(doc, "div", null, "stat-card");
    card1.appendChild(el(doc, "div", "Stored upgrade verdict", "stat-label"));
    var v1 = el(doc, "div", null, "stat-value");
    v1.appendChild(badge(doc, verdict.verdict, verdict.cls));
    card1.appendChild(v1);
    var s1 = el(doc, "div", formatMinors(verdict.minors) + " ", "stat-sub");
    var fullLink = el(doc, "a", "Open full report");
    fullLink.href = upgradePageURL(ctx.rootPath, ctx.repo, ctx.chart, ctx.from, ctx.to);
    s1.appendChild(fullLink);
    card1.appendChild(s1);
    grid.appendChild(card1);

    var card2 = el(doc, "div", null, "stat-card");
    card2.appendChild(el(doc, "div", "Values keys in report", "stat-label"));
    card2.appendChild(el(doc, "div", String(totalValKeys), "stat-value"));
    card2.appendChild(el(doc, "div", ctx.paths.length + " keys read from your paste", "stat-sub"));
    grid.appendChild(card2);

    var card3 = el(doc, "div", null, "stat-card");
    card3.appendChild(el(doc, "div", hasPaste ? "Needs review" : "Values status", "stat-label"));
    if (hasPaste) {
      card3.appendChild(el(doc, "div", String(attention), "stat-value"));
      card3.appendChild(el(doc, "div", covered + " covered or pinned by your values", "stat-sub"));
    } else {
      card3.appendChild(el(doc, "div", "Paste YAML", "stat-value"));
      card3.appendChild(el(doc, "div", "Paste values.yaml to see what affects you", "stat-sub"));
    }
    grid.appendChild(card3);

    host.appendChild(grid);

    if (totalValKeys === 0) {
      var empty = el(doc, "div", null, "panel");
      empty.appendChild(el(doc, "p", "This stored upgrade report recorded no dropped, renamed, required-unset, or changed-default values keys for chart defaults."));
      empty.appendChild(el(doc, "p", "To check keys that only exist in your custom values.yaml, run the CLI command below locally.", "muted"));
      host.appendChild(empty);
    }

    function addSection(title, subtitle, rows, describe) {
      if (!rows.length) {
        return;
      }
      var ordered = rows.slice().sort(function (x, y) {
        var sx = describe(x).rank;
        var sy = describe(y).rank;
        if (sx !== sy) {
          return sx - sy;
        }
        return x.resource.localeCompare(y.resource);
      });
      host.appendChild(el(doc, "h2", title + " (" + ordered.length + ")"));
      if (subtitle) {
        host.appendChild(el(doc, "p", subtitle, "muted"));
      }
      var list = el(doc, "ul", null, "finding-list");
      for (var i = 0; i < ordered.length; i++) {
        var row = ordered[i];
        var info = describe(row);
        var li = el(doc, "li", null, "finding-card state-" + info.state);
        var head = el(doc, "div", null, "finding-head");
        head.appendChild(el(doc, "code", row.resource, "finding-key"));
        head.appendChild(badge(doc, info.label, info.badgeKind));
        head.appendChild(el(doc, "span", formatMinors(row.minors), "muted"));
        li.appendChild(head);
        if (row.message) {
          li.appendChild(el(doc, "p", row.message, "finding-msg"));
        }
        if (row.detail) {
          li.appendChild(el(doc, "p", row.detail, "finding-detail"));
        }
        list.appendChild(li);
      }
      host.appendChild(list);
    }

    addSection(
      "Dropped keys",
      "Keys removed from the new chart.",
      groups.dropped,
      function (r) {
        if (!hasPaste) {
          return { rank: 1, state: "warn", badgeKind: "neutral", label: "Paste YAML to check" };
        }
        return r.hit
          ? { rank: 0, state: "bad", badgeKind: "bad", label: "Set in your values" }
          : { rank: 2, state: "ok", badgeKind: "neutral", label: "Not in your values" };
      }
    );

    addSection(
      "Renamed keys",
      "Keys moved to a new path in the new chart.",
      groups.renamed,
      function (r) {
        if (!hasPaste) {
          return { rank: 1, state: "warn", badgeKind: "neutral", label: "Paste YAML to check" };
        }
        return r.hit
          ? { rank: 0, state: "bad", badgeKind: "bad", label: "Set in your values" }
          : { rank: 2, state: "ok", badgeKind: "neutral", label: "Not in your values" };
      }
    );

    addSection(
      "New required keys",
      "Required by the new chart and left unset by chart defaults.",
      groups.required,
      function (r) {
        if (!hasPaste) {
          return { rank: 0, state: "bad", badgeKind: "bad", label: "Required by upgrade" };
        }
        return r.hit
          ? { rank: 2, state: "ok", badgeKind: "ok", label: "Set in your values" }
          : { rank: 0, state: "bad", badgeKind: "bad", label: "Missing from your values" };
      }
    );

    addSection(
      "Changed chart defaults",
      "Defaults that changed between these versions and alter rendered Kubernetes objects. If your values.yaml does not override a key, your cluster inherits the new default.",
      groups.changed,
      function (r) {
        if (!hasPaste) {
          return { rank: 1, state: "warn", badgeKind: "warn", label: "Default changed" };
        }
        return r.hit
          ? { rank: 2, state: "ok", badgeKind: "ok", label: "Pinned by your values" }
          : { rank: 0, state: "warn", badgeKind: "warn", label: "Inherits new default" };
      }
    );

    if (groups.blockers.length > 0) {
      host.appendChild(el(doc, "h2", "Other upgrade blockers (" + groups.blockers.length + ")"));
      host.appendChild(el(doc, "p", "Chart-level blockers recorded for this version jump (not specific to a values key).", "muted"));
      var blist = el(doc, "ul", null, "finding-list");
      for (var k = 0; k < groups.blockers.length; k++) {
        var br = groups.blockers[k];
        var bli = el(doc, "li", null, "finding-card state-bad");
        var bhead = el(doc, "div", null, "finding-head");
        bhead.appendChild(el(doc, "code", br.rule, "finding-key"));
        bhead.appendChild(badge(doc, "Blocker", "bad"));
        bhead.appendChild(el(doc, "span", formatMinors(br.minors), "muted"));
        bli.appendChild(bhead);
        if (br.resource) {
          bli.appendChild(el(doc, "p", br.resource, "finding-detail"));
        }
        if (br.message) {
          bli.appendChild(el(doc, "p", br.message, "finding-msg"));
        }
        blist.appendChild(bli);
      }
      host.appendChild(blist);
    }

    var cmd = cliWithValues(ctx.report && ctx.report.reproduce_command);
    if (cmd) {
      var box = el(doc, "div", null, "cli-box");
      var chead = el(doc, "div", null, "cli-head");
      chead.appendChild(el(doc, "strong", "Run the full check with your values.yaml"));
      var copyBtn = el(doc, "button", "Copy command", "btn btn-secondary btn-sm");
      copyBtn.type = "button";
      copyBtn.addEventListener("click", function () {
        if (typeof navigator !== "undefined" && navigator.clipboard && navigator.clipboard.writeText) {
          navigator.clipboard.writeText(cmd).then(function () {
            copyBtn.textContent = "Copied";
            setTimeout(function () { copyBtn.textContent = "Copy command"; }, 1500);
          }).catch(function () {});
        }
      });
      chead.appendChild(copyBtn);
      box.appendChild(chead);
      box.appendChild(el(doc, "pre", cmd));
      host.appendChild(box);
    }
  }

  function boot(doc) {
    var button = doc.getElementById("run");
    if (!button) {
      return;
    }
    var rootPath = doc.body.getAttribute("data-root") || "";
    var status = doc.getElementById("status");
    var out = doc.getElementById("out");
    var select = doc.getElementById("chart");
    var search = doc.getElementById("chart-search");
    var chartCount = doc.getElementById("chart-count");
    var fromSel = doc.getElementById("from");
    var toSel = doc.getElementById("to");
    var pairMeta = doc.getElementById("pair-meta");
    var yamlBox = doc.getElementById("yaml");
    var keyCount = doc.getElementById("key-count");
    var loadFileBtn = doc.getElementById("load-file");
    var yamlFileInput = doc.getElementById("yaml-file");
    var clearBtn = doc.getElementById("clear-yaml");

    var charts = [];
    var chartCache = {};
    var reportCache = {};
    var currentChartDoc = null;
    var loadSeq = 0;

    var params = new URLSearchParams(doc.location.search);
    var preset = {
      repo: params.get("repo") || "",
      chart: params.get("chart") || "",
      from: params.get("from") || "",
      to: params.get("to") || "",
      applied: false
    };

    function say(text, isError) {
      status.textContent = text;
      status.className = isError ? "status error" : "status";
    }

    function updateKeyCount() {
      var n = keyPaths(yamlBox.value).length;
      keyCount.textContent = n + (n === 1 ? " key in paste" : " keys in paste");
    }

    function selectedChartParts() {
      var val = select.value || "";
      if (!val) {
        return null;
      }
      var parts = val.split("\n");
      if (!parts[0] || !parts[1]) {
        return null;
      }
      return { repo: parts[0], chart: parts[1] };
    }

    function updatePairMeta() {
      pairMeta.textContent = "";
      if (!currentChartDoc || !fromSel.value || !toSel.value) {
        pairMeta.hidden = true;
        return;
      }
      var match = null;
      for (var i = 0; i < currentChartDoc.pairs.length; i++) {
        var p = currentChartDoc.pairs[i];
        if (p.from === fromSel.value && p.to === toSel.value) {
          match = p;
          break;
        }
      }
      if (!match) {
        pairMeta.hidden = true;
        return;
      }
      pairMeta.hidden = false;
      var statusLabel = match.status === "blocked"
        ? "Blocked"
        : (match.status === "unchecked" ? "Not checked" : "No blocking finding");
      var badgeKind = match.status === "blocked" ? "bad" : (match.status === "unchecked" ? "neutral" : "ok");
      pairMeta.appendChild(badge(doc, statusLabel, badgeKind));
      if (match.values_keys) {
        pairMeta.appendChild(badge(doc, match.values_keys + (match.values_keys === 1 ? " values key" : " values keys"), "warn"));
      } else {
        pairMeta.appendChild(el(doc, "span", "No values-key findings recorded for chart defaults", "muted"));
      }
      var link = el(doc, "a", "View full upgrade report");
      link.href = upgradePageURL(rootPath, currentChartDoc.repo_name, currentChartDoc.chart, match.from, match.to);
      pairMeta.appendChild(link);
    }

    function populateTo(from, preferredTo) {
      toSel.textContent = "";
      if (!currentChartDoc) {
        toSel.disabled = true;
        updatePairMeta();
        return;
      }
      var targets = targetsForFrom(currentChartDoc.pairs, from);
      for (var i = 0; i < targets.length; i++) {
        var t = targets[i];
        var label = t.to;
        if (t.adjacent) {
          label += " (next release)";
        }
        var opt = el(doc, "option", label);
        opt.value = t.to;
        toSel.appendChild(opt);
      }
      toSel.disabled = targets.length === 0;
      if (preferredTo) {
        for (var j = 0; j < toSel.options.length; j++) {
          if (toSel.options[j].value === preferredTo) {
            toSel.selectedIndex = j;
            break;
          }
        }
      }
      updatePairMeta();
    }

    function applyChartDoc(docData, wantFrom, wantTo) {
      currentChartDoc = docData;
      fromSel.textContent = "";
      var froms = uniqueFroms(docData.pairs || []);
      for (var i = 0; i < froms.length; i++) {
        var opt = el(doc, "option", froms[i]);
        opt.value = froms[i];
        fromSel.appendChild(opt);
      }
      fromSel.disabled = froms.length === 0;
      var chosen = pickDefaultPair(docData, wantFrom, wantTo);
      for (var j = 0; j < fromSel.options.length; j++) {
        if (fromSel.options[j].value === chosen.from) {
          fromSel.selectedIndex = j;
          break;
        }
      }
      populateTo(fromSel.value, chosen.to);
    }

    function loadSelectedChart(wantFrom, wantTo) {
      var parts = selectedChartParts();
      if (!parts) {
        currentChartDoc = null;
        fromSel.textContent = "";
        toSel.textContent = "";
        fromSel.disabled = true;
        toSel.disabled = true;
        pairMeta.hidden = true;
        return;
      }
      var url;
      try {
        url = chartURL(rootPath, parts.repo, parts.chart);
      } catch (err) {
        say(err.message, true);
        return;
      }
      if (chartCache[url]) {
        applyChartDoc(chartCache[url], wantFrom, wantTo);
        return;
      }
      var seq = ++loadSeq;
      fromSel.disabled = true;
      toSel.disabled = true;
      fromSel.textContent = "";
      fromSel.appendChild(el(doc, "option", "Loading versions..."));
      toSel.textContent = "";
      toSel.appendChild(el(doc, "option", "Loading versions..."));
      pairMeta.hidden = true;

      fetch(url, { method: "GET", credentials: "same-origin" })
        .then(function (res) {
          if (!res.ok) {
            throw new Error("Could not load versions for " + parts.repo + "/" + parts.chart + ".");
          }
          return res.json();
        })
        .then(function (data) {
          chartCache[url] = data;
          if (seq !== loadSeq) {
            return;
          }
          applyChartDoc(data, wantFrom, wantTo);
        })
        .catch(function (err) {
          if (seq !== loadSeq) {
            return;
          }
          say(err.message || "Could not load chart versions.", true);
        });
    }

    function fillCharts(skipLoad) {
      var prevValue = select.value;
      var tokens = (search.value || "").toLowerCase().trim().split(/\s+/).filter(Boolean);
      select.textContent = "";
      var matched = 0;
      for (var i = 0; i < charts.length; i++) {
        var c = charts[i];
        var hay = ((c.id || "") + " " + (c.repo_name || "") + " " + (c.chart || "") + " " + (c.repo || "")).toLowerCase();
        var ok = true;
        for (var t = 0; t < tokens.length; t++) {
          if (hay.indexOf(tokens[t]) < 0) {
            ok = false;
            break;
          }
        }
        if (!ok) {
          continue;
        }
        var label = c.id + (c.latest ? " (latest " + c.latest + ")" : "");
        var opt = el(doc, "option", label);
        opt.value = (c.repo_name || "") + "\n" + (c.chart || "");
        select.appendChild(opt);
        matched++;
      }
      if (!tokens.length) {
        chartCount.textContent = charts.length + " indexed charts";
      } else if (matched === 0) {
        chartCount.textContent = "No charts match";
        var none = el(doc, "option", "No matching charts");
        none.value = "";
        select.appendChild(none);
      } else {
        chartCount.textContent = matched + " of " + charts.length + " matching";
      }
      var kept = false;
      if (prevValue) {
        for (var j = 0; j < select.options.length; j++) {
          if (select.options[j].value === prevValue) {
            select.selectedIndex = j;
            kept = true;
            break;
          }
        }
      }
      if (!kept && !skipLoad) {
        loadSelectedChart("", "");
      }
    }

    function runCheck() {
      out.textContent = "";
      var parts = selectedChartParts();
      if (!parts) {
        say("Choose a chart first.", true);
        return;
      }
      var from = fromSel.value;
      var to = toSel.value;
      if (!from || !to) {
        say("Choose both a From version and a To version.", true);
        return;
      }
      var url;
      try {
        url = reportURL(rootPath, parts.repo, parts.chart, from, to);
      } catch (err) {
        say(err.message, true);
        return;
      }
      var yaml = yamlBox.value;
      var paths = keyPaths(yaml);

      function show(report) {
        say("Checked " + paths.length + " YAML keys locally against " + parts.repo + "/" + parts.chart + " " + from + " -> " + to + ". Nothing was uploaded.", false);
        renderResults(doc, out, {
          rootPath: rootPath,
          repo: parts.repo,
          chart: parts.chart,
          from: from,
          to: to,
          paths: paths,
          report: report,
          groups: intersect(paths, report)
        });
      }

      if (reportCache[url]) {
        show(reportCache[url]);
        return;
      }

      say("Loading stored upgrade report...", false);
      fetch(url, { method: "GET", credentials: "same-origin" })
        .then(function (res) {
          if (!res.ok) {
            throw new Error("This site has no stored report for that version pair.");
          }
          return res.json();
        })
        .then(function (report) {
          reportCache[url] = report;
          show(report);
        })
        .catch(function (err) {
          say(err.message || "The report did not load.", true);
        });
    }

    fetch(rootPath + "/api/v1/index.json", { method: "GET", credentials: "same-origin" })
      .then(function (res) {
        if (!res.ok) {
          throw new Error("Could not load chart catalog.");
        }
        return res.json();
      })
      .then(function (cat) {
        var all = (cat && cat.charts) || [];
        charts = all.filter(function (c) {
          return (c.pairs || 0) > 0;
        }).sort(function (a, b) {
          var sa = a.stars || 0;
          var sb = b.stars || 0;
          if (sa !== sb) {
            return sb - sa;
          }
          return (a.id || "").localeCompare(b.id || "");
        });
        fillCharts(true);
        if (preset.repo && preset.chart && !preset.applied) {
          preset.applied = true;
          var wantVal = preset.repo + "\n" + preset.chart;
          for (var i = 0; i < select.options.length; i++) {
            if (select.options[i].value === wantVal) {
              select.selectedIndex = i;
              break;
            }
          }
          loadSelectedChart(preset.from, preset.to);
        } else {
          loadSelectedChart("", "");
        }
      })
      .catch(function () {
        chartCount.textContent = "Unavailable";
        say("Could not load the chart list from this site.", true);
      });

    search.addEventListener("input", function () {
      fillCharts(false);
    });
    select.addEventListener("change", function () {
      out.textContent = "";
      say("", false);
      loadSelectedChart("", "");
    });
    fromSel.addEventListener("change", function () {
      out.textContent = "";
      populateTo(fromSel.value, "");
    });
    toSel.addEventListener("change", function () {
      out.textContent = "";
      updatePairMeta();
    });
    yamlBox.addEventListener("input", updateKeyCount);
    yamlBox.addEventListener("keydown", function (ev) {
      if ((ev.ctrlKey || ev.metaKey) && ev.key === "Enter") {
        ev.preventDefault();
        runCheck();
      }
    });

    if (loadFileBtn && yamlFileInput) {
      loadFileBtn.addEventListener("click", function () {
        yamlFileInput.click();
      });
      yamlFileInput.addEventListener("change", function () {
        var file = yamlFileInput.files && yamlFileInput.files[0];
        if (!file) {
          return;
        }
        var reader = new FileReader();
        reader.onload = function () {
          yamlBox.value = String(reader.result || "");
          updateKeyCount();
          say("Loaded " + file.name + " in your browser. Nothing was uploaded.", false);
        };
        reader.readAsText(file);
        yamlFileInput.value = "";
      });
    }

    yamlBox.addEventListener("dragover", function (ev) {
      ev.preventDefault();
      yamlBox.classList.add("drag-over");
    });
    yamlBox.addEventListener("dragleave", function () {
      yamlBox.classList.remove("drag-over");
    });
    yamlBox.addEventListener("drop", function (ev) {
      ev.preventDefault();
      yamlBox.classList.remove("drag-over");
      var file = ev.dataTransfer && ev.dataTransfer.files && ev.dataTransfer.files[0];
      if (!file) {
        return;
      }
      var reader = new FileReader();
      reader.onload = function () {
        yamlBox.value = String(reader.result || "");
        updateKeyCount();
        say("Loaded " + file.name + " in your browser. Nothing was uploaded.", false);
      };
      reader.readAsText(file);
    });

    if (clearBtn) {
      clearBtn.addEventListener("click", function () {
        yamlBox.value = "";
        out.textContent = "";
        updateKeyCount();
        say("", false);
      });
    }

    button.addEventListener("click", runCheck);
  }

  return {
    keyPaths: keyPaths,
    intersect: intersect,
    formatPath: formatPath,
    chartURL: chartURL,
    reportURL: reportURL,
    uniqueFroms: uniqueFroms,
    targetsForFrom: targetsForFrom,
    pickDefaultPair: pickDefaultPair,
    cliWithValues: cliWithValues,
    boot: boot
  };
});
