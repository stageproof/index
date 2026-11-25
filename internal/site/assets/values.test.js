const test = require("node:test");
const assert = require("node:assert/strict");
const values = require("./values.js");

test("nested maps become dotted paths", function () {
  const paths = values.keyPaths("auth:\n  teleportConfig:\n    extra: 1\nclusterName: demo\n");
  assert.deepEqual(paths, ["auth", "auth.teleportConfig", "auth.teleportConfig.extra", "clusterName"]);
});

test("a dotted key stays one segment", function () {
  const paths = values.keyPaths('"a.b": 1\nnested:\n  "c.d": 2\n');
  assert.deepEqual(paths, ['["a.b"]', 'nested', 'nested["c.d"]']);
});

test("anchors and merge keys are copied", function () {
  const yaml = [
    "defaults: &base",
    "  size: 1",
    "prod:",
    "  <<: *base",
    "  name: web",
    "copy: *base"
  ].join("\n");
  const paths = values.keyPaths(yaml);
  assert.deepEqual(paths, ["copy", "copy.size", "defaults", "defaults.size", "prod", "prod.name", "prod.size"]);
});

test("a list of maps keeps the child key", function () {
  const paths = values.keyPaths("ingress:\n  hosts:\n    - host: example.com\n");
  assert.ok(paths.indexOf("ingress.hosts") >= 0);
  assert.ok(paths.indexOf("ingress.hosts.host") >= 0);
  assert.ok(paths.indexOf("ingress.hosts[0].host") >= 0);
});

test("intersection uses finding resources and ignores other rules", function () {
  const paths = values.keyPaths("auth:\n  teleportConfig: yes\nsize: 1\n");
  const report = {
    k8s_results: {
      "1.35": {
        findings: [
          { rule_id: "helm.values.silently_dropped", resource: "size", message: "size dropped" },
          { rule_id: "helm.values.renamed", resource: "old.name", message: "renamed" },
          { rule_id: "helm.values.default_changed", resource: "image.tag", message: "tag" },
          { rule_id: "helm.values.new_required_unset", resource: "auth.teleportConfig", message: "required" },
          { rule_id: "helm.values.new_required_unset", resource: "clusterName", message: "required", ignored: true },
          { rule_id: "k8s.api.removed", resource: "policy/v1beta1", message: "removed" }
        ]
      }
    }
  };
  const got = values.intersect(paths, report);
  assert.equal(got.dropped.length, 1);
  assert.equal(got.dropped[0].hit, true);
  assert.equal(got.renamed[0].hit, false);
  assert.equal(got.changed[0].hit, false);
  assert.equal(got.required.length, 1);
  assert.equal(got.required[0].hit, true);
});

test("a nested paste hits the parent key the report recorded", function () {
  const paths = values.keyPaths("config:\n  logLevel: debug\n");
  const got = values.intersect(paths, {
    k8s_results: {
      "1.35": {
        findings: [{ rule_id: "helm.values.silently_dropped", resource: "config.logLevel", message: "dropped" }]
      }
    }
  });
  assert.equal(got.dropped[0].hit, true);
});

test("report URLs stay on this site", function () {
  assert.equal(
    values.reportURL("/index", "fixtures", "web", "1.0.0", "1.2.0+build"),
    "/index/api/v1/upgrade/fixtures/web/1.0.0/1.2.0+build.json"
  );
  assert.throws(function () {
    values.reportURL("/index", "../etc", "web", "1", "2");
  });
});
