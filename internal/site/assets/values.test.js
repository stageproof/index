const test = require("node:test");
const assert = require("node:assert/strict");
const values = require("./values.js");

test("nested maps become dotted paths", function () {
  const paths = values.keyPaths("auth:\n  teleportConfig:\n    extra: 1\nclusterName: demo\n");
  assert.deepEqual(paths, ["auth", "auth.teleportConfig", "auth.teleportConfig.extra", "clusterName"]);
});

test("a dotted key stays one segment", function () {
  const paths = values.keyPaths('"a.b": 1\nnested:\n  "c.d": 2\n');
  assert.deepEqual(paths, ['["a.b"]', "nested", 'nested["c.d"]']);
});

test("anchors and merge keys are copied", function () {
  const yaml = [
    "defaults: &base",
    "  size: 1",
    "extra: &more",
    "  tier: app",
    "prod:",
    "  <<: [*base, *more]",
    "  name: web",
    "copy: *base"
  ].join("\n");
  const paths = values.keyPaths(yaml);
  assert.deepEqual(paths, [
    "copy",
    "copy.size",
    "defaults",
    "defaults.size",
    "extra",
    "extra.tier",
    "prod",
    "prod.name",
    "prod.size",
    "prod.tier"
  ]);
});

test("block scalars do not leak fake child keys", function () {
  const yaml = [
    "config: |-",
    "  nested: should_not_be_a_key",
    "  another: value",
    "replicaCount: 2"
  ].join("\n");
  const paths = values.keyPaths(yaml);
  assert.deepEqual(paths, ["config", "replicaCount"]);
});

test("a list of maps keeps the child key", function () {
  const paths = values.keyPaths("ingress:\n  hosts:\n    - host: example.com\n");
  assert.ok(paths.indexOf("ingress.hosts") >= 0);
  assert.ok(paths.indexOf("ingress.hosts.host") >= 0);
  assert.ok(paths.indexOf("ingress.hosts[0].host") >= 0);
});

test("intersection deduplicates across K8s minors and keeps chart blockers", function () {
  const paths = values.keyPaths("auth:\n  teleportConfig: yes\nsize: 1\n");
  const report = {
    k8s_results: {
      "1.34": {
        verdict: "BLOCKED",
        findings: [
          { rule_id: "helm.values.silently_dropped", resource: "size", message: "size dropped" },
          { rule_id: "helm.values.default_changed", resource: "image.tag", message: "tag changed", severity: "warning" },
          { rule_id: "helm.values.new_required_unset", resource: "auth.teleportConfig", message: "required", severity: "blocker" },
          { rule_id: "k8s.api.removed", resource: "policy/v1beta1", message: "removed", severity: "blocker" }
        ]
      },
      "1.35": {
        verdict: "BLOCKED",
        findings: [
          { rule_id: "helm.values.silently_dropped", resource: "size", message: "size dropped" },
          { rule_id: "helm.values.renamed", resource: "old.name", message: "renamed" },
          { rule_id: "helm.values.default_changed", resource: "image.tag", message: "tag changed", severity: "warning" },
          { rule_id: "helm.values.new_required_unset", resource: "auth.teleportConfig", message: "required", severity: "blocker" },
          { rule_id: "helm.values.new_required_unset", resource: "clusterName", message: "required", ignored: true },
          { rule_id: "k8s.api.removed", resource: "policy/v1beta1", message: "removed", severity: "blocker" }
        ]
      }
    }
  };
  const got = values.intersect(paths, report);
  assert.equal(got.dropped.length, 1);
  assert.deepEqual(got.dropped[0].minors, ["1.34", "1.35"]);
  assert.equal(got.dropped[0].hit, true);
  assert.equal(got.renamed.length, 1);
  assert.equal(got.renamed[0].hit, false);
  assert.equal(got.changed.length, 1);
  assert.equal(got.changed[0].hit, false);
  assert.equal(got.required.length, 1);
  assert.equal(got.required[0].hit, true);
  assert.equal(got.blockers.length, 1);
  assert.deepEqual(got.blockers[0].minors, ["1.34", "1.35"]);
});

test("version picker defaults to latest against previous and filters targets", function () {
  const doc = {
    default_from: "2.0.0",
    default_to: "2.1.0",
    pairs: [
      { from: "2.0.0", to: "2.1.0", adjacent: true },
      { from: "1.9.0", to: "2.1.0" },
      { from: "1.9.0", to: "2.0.0", adjacent: true }
    ]
  };
  assert.deepEqual(values.pickDefaultPair(doc, "", ""), { from: "2.0.0", to: "2.1.0" });
  assert.deepEqual(values.pickDefaultPair(doc, "1.9.0", "2.1.0"), { from: "1.9.0", to: "2.1.0" });
  assert.deepEqual(values.uniqueFroms(doc.pairs), ["2.0.0", "1.9.0"]);
  assert.deepEqual(values.targetsForFrom(doc.pairs, "1.9.0").map(function (p) { return p.to; }), ["2.1.0", "2.0.0"]);
});

test("chart and report URLs stay on this site", function () {
  assert.equal(
    values.chartURL("/index", "fixtures", "web"),
    "/index/api/v1/chart/fixtures/web.json"
  );
  assert.equal(
    values.reportURL("/index", "fixtures", "web", "1.0.0", "1.2.0+build"),
    "/index/api/v1/upgrade/fixtures/web/1.0.0/1.2.0+build.json"
  );
  assert.throws(function () {
    values.chartURL("/index", "../etc", "web");
  });
  assert.throws(function () {
    values.reportURL("/index", "../etc", "web", "1", "2");
  });
});

test("cliWithValues appends -f values.yaml once", function () {
  assert.equal(
    values.cliWithValues("stageproof check --chart web"),
    "stageproof check --chart web -f values.yaml"
  );
  assert.equal(
    values.cliWithValues("stageproof check --chart web -f custom.yaml"),
    "stageproof check --chart web -f custom.yaml"
  );
});
