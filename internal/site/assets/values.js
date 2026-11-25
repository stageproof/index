// Browser-only values check. The paste is read in this page and is not sent.
// A stored report is chart defaults. Key lists come from findings already in that JSON.

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
        if (c === quote && (quote !== "'" || line[i - 1] !== "\\")) {
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
      return JSON.parse(raw);
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
    if (src === "" || src === "|" || src === ">" || src === "|-" || src === ">-") {
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
      var colon = src.indexOf(":");
      if (colon <= 0) {
        return null;
      }
      key = src.slice(0, colon).trim();
      rest = src.slice(colon + 1).trim();
    }
    var anchor = "";
    var alias = "";
    if (rest.startsWith("&")) {
      var parts = rest.split(/\s+/);
      anchor = parts[0].slice(1);
      rest = parts.slice(1).join(" ");
    }
    if (rest.startsWith("*")) {
      alias = rest.slice(1).split(/\s+/)[0];
      rest = "";
    }
    var flow = "";
    if (rest.startsWith("{") && rest.endsWith("}")) {
      flow = rest;
      rest = "";
    }
    return {
      key: key,
      anchor: anchor,
      alias: alias,
      flow: flow,
      nested: rest === "" || rest === "|" || rest === ">" || rest === "|-" || rest === ">-" || rest === "null"
    };
  }

  function linesOf(text) {
    var rows = String(text).replace(/^\uFEFF/, "").replace(/\r\n/g, "\n").split("\n");
    var out = [];
    for (var i = 0; i < rows.length; i++) {
      var raw = stripComment(rows[i]);
      if (raw.trim() === "" || raw.trim() === "---" || raw.trim() === "...") {
        continue;
      }
      var indent = 0;
      while (raw[indent] === " ") {
        indent++;
      }
      while (raw[indent] === "\t") {
        indent += 2;
      }
      out.push({ indent: indent, text: raw.trim() });
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

  function parseSeq(lines, i, indent, anchors) {
    var items = [];
    while (i < lines.length && lines[i].indent === indent && lines[i].text.startsWith("- ")) {
      var rest = lines[i].text.slice(2).trim();
      var item = { kind: "scalar" };
      i++;
      if (rest === "") {
        if (i < lines.length && lines[i].indent > indent && !lines[i].text.startsWith("- ")) {
          var nested = parseMap(lines, i, lines[i].indent, anchors);
          item = nested.node;
          i = nested.next;
        }
      } else if (splitKey(rest)) {
        var synthetic = [{ indent: indent + 2, text: rest }];
        var j = i;
        while (j < lines.length && lines[j].indent > indent && !lines[j].text.startsWith("- ")) {
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
      if (lines[i].indent < indent) {
        break;
      }
      if (lines[i].indent > indent) {
        break;
      }
      if (lines[i].text.startsWith("- ")) {
        break;
      }
      var item = splitKey(lines[i].text);
      i++;
      if (!item) {
        continue;
      }
      var value = { kind: "scalar" };
      if (item.alias) {
        value = anchors[item.alias] ? clone(anchors[item.alias]) : { kind: "scalar" };
      } else if (item.flow) {
        value = parseFlow(item.flow, anchors);
      } else if (item.nested && i < lines.length && lines[i].indent > indent) {
        if (lines[i].text.startsWith("- ")) {
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
    var groups = { dropped: [], renamed: [], changed: [], required: [] };
    var results = (report && report.k8s_results) || {};
    var minors = Object.keys(results).sort();
    for (var m = 0; m < minors.length; m++) {
      var findings = results[minors[m]].findings || [];
      for (var i = 0; i < findings.length; i++) {
        var f = findings[i];
        if (!f || f.ignored || !f.resource || !RULES[f.rule_id]) {
          continue;
        }
        groups[RULES[f.rule_id]].push({
          minor: minors[m],
          resource: f.resource,
          message: f.message || "",
          rule: f.rule_id,
          hit: hits(paths, f.resource)
        });
      }
    }
    return groups;
  }

  function segment(s) {
    return typeof s === "string" && s !== "" && s !== "." && s !== ".." && s.indexOf("..") < 0 && !/[\/\\?#]/.test(s);
  }

  function reportURL(root, repo, chart, from, to) {
    var parts = [repo, chart, from, to];
    for (var i = 0; i < parts.length; i++) {
      if (!segment(parts[i])) {
        throw new Error("That name cannot be used in a path.");
      }
    }
    var path = root + "/api/v1/upgrade/" + parts.map(function (s) {
      return encodeURIComponent(s).replace(/%2B/g, "+");
    }).join("/") + ".json";
    if (path.indexOf(root + "/api/v1/") !== 0) {
      throw new Error("Refusing a report URL outside this site.");
    }
    return path;
  }

  function el(doc, tag, text) {
    var node = doc.createElement(tag);
    if (text !== undefined) {
      node.textContent = text;
    }
    return node;
  }

  function render(doc, host, groups) {
    host.textContent = "";
    var names = [
      ["dropped", "Dropped keys"],
      ["renamed", "Renamed keys"],
      ["changed", "Changed defaults"],
      ["required", "Required keys the chart defaults leave unset"]
    ];
    var any = false;
    for (var n = 0; n < names.length; n++) {
      if (groups[names[n][0]].length) {
        any = true;
      }
    }
    if (!any) {
      host.appendChild(el(doc, "p", "This stored report has no dropped, renamed, changed-default, or required-unset values keys. A chart-defaults check usually does not record a key that only your file sets."));
      return;
    }
    for (var i = 0; i < names.length; i++) {
      var key = names[i][0];
      var rows = groups[key];
      var title = el(doc, "h2", names[i][1]);
      host.appendChild(title);
      if (!rows.length) {
        host.appendChild(el(doc, "p", "None in this report."));
        continue;
      }
      var list = el(doc, "ul");
      for (var r = 0; r < rows.length; r++) {
        var row = rows[r];
        var note;
        if (key === "required") {
          note = row.hit ? "Your paste sets this key." : "Your paste does not set this key.";
        } else if (row.hit) {
          note = "Your paste sets this key.";
        } else {
          note = "Your paste does not set this key.";
        }
        var item = el(doc, "li");
        item.appendChild(el(doc, "code", row.resource));
        item.appendChild(doc.createTextNode(" (" + row.minor + "). " + note + (row.message ? " " + row.message : "")));
        list.appendChild(item);
      }
      host.appendChild(list);
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
    var charts = [];
    var params = new URLSearchParams(doc.location.search);
    var preset = {
      repo: params.get("repo") || "",
      chart: params.get("chart") || "",
      from: params.get("from") || "",
      to: params.get("to") || ""
    };
    if (preset.from) {
      doc.getElementById("from").value = preset.from;
    }
    if (preset.to) {
      doc.getElementById("to").value = preset.to;
    }

    function say(text) {
      status.textContent = text;
    }

    function fill() {
      var q = (search.value || "").toLowerCase();
      select.textContent = "";
      for (var i = 0; i < charts.length; i++) {
        var id = charts[i].id || "";
        if (q && id.toLowerCase().indexOf(q) < 0) {
          continue;
        }
        var opt = el(doc, "option", id);
        opt.value = (charts[i].repo_name || "") + "\n" + (charts[i].chart || "");
        select.appendChild(opt);
      }
    }

    fetch(rootPath + "/api/v1/index.json", { method: "GET", credentials: "same-origin" })
      .then(function (res) {
        if (!res.ok) {
          throw new Error("catalog");
        }
        return res.json();
      })
      .then(function (cat) {
        charts = (cat && cat.charts) || [];
        fill();
        var want = preset.repo + "/" + preset.chart;
        for (var i = 0; i < select.options.length; i++) {
          if (select.options[i].textContent === want) {
            select.selectedIndex = i;
            break;
          }
        }
      })
      .catch(function () {
        say("The chart list did not load. The comparison still runs when you type a pair this site has.");
      });

    search.addEventListener("input", fill);

    button.addEventListener("click", function () {
      out.textContent = "";
      var chosen = (select.value || "").split("\n");
      var repo = chosen[0] || preset.repo;
      var chart = chosen[1] || preset.chart;
      var from = doc.getElementById("from").value.trim();
      var to = doc.getElementById("to").value.trim();
      var yaml = doc.getElementById("yaml").value;
      if (!repo || !chart) {
        say("Choose a chart.");
        return;
      }
      var url;
      try {
        url = reportURL(rootPath, repo, chart, from, to);
      } catch (err) {
        say(err.message);
        return;
      }
      say("Loading the stored report.");
      fetch(url, { method: "GET", credentials: "same-origin" })
        .then(function (res) {
          if (!res.ok) {
            throw new Error("This site has no stored report for that pair.");
          }
          return res.json();
        })
        .then(function (report) {
          var paths = keyPaths(yaml);
          say(paths.length + " keys read from the paste. Nothing was uploaded.");
          render(doc, out, intersect(paths, report));
        })
        .catch(function (err) {
          say(err.message || "The report did not load.");
        });
    });
  }

  return {
    keyPaths: keyPaths,
    intersect: intersect,
    formatPath: formatPath,
    reportURL: reportURL,
    boot: boot
  };
});
