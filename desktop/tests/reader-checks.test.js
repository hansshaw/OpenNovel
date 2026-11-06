"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const { waitForPDFCanvas, focusReaderWindow, waitForNativeFullscreen } = require("../scripts/reader-checks.js");

const pending = { canvas: { width: 400, height: 500 }, note: "正在绘制 PDF 原页…", hidden: false };
const finished = { ...pending, note: "" };
function clock() { let time = 0; return { now: () => time, sleep: async ms => { time += ms; } }; }

test("fullscreen preparation waits for renderer focus even after the native focus request returns", async () => {
  const actions = []; let focusAttempts = 0;
  const window = {
    show() { actions.push("show"); }, focus() { actions.push("native-focus"); },
    webContents: {
      focus() { focusAttempts++; },
      executeJavaScript: async () => focusAttempts >= 3,
    },
  };
  await focusReaderWindow(window, clock());
  assert.deepEqual(actions, ["show", "native-focus"]);
  assert.equal(focusAttempts, 3);
});

test("a document that cannot gain focus fails instead of issuing a fullscreen request", async () => {
  let attempts = 0;
  const window = { show() {}, focus() {}, webContents: { focus() { attempts++; }, executeJavaScript: async () => false } };
  await assert.rejects(focusReaderWindow(window, { ...clock(), timeoutMs: 300 }), /did not receive focus/);
  assert.equal(attempts, 3);
});

test("fullscreen checks wait for both native entry and native exit and never accept a stuck transition", async () => {
  for (const expected of [true, false]) {
    let reads = 0;
    await waitForNativeFullscreen({ isFullScreen: () => ++reads >= 3 ? expected : !expected }, expected, clock());
    assert.equal(reads, 3);
    await assert.rejects(waitForNativeFullscreen({ isFullScreen: () => !expected }, expected, { ...clock(), timeoutMs: 90 }), expected ? /did not enter/ : /did not leave/);
  }
});

test("PDF canvas readiness does not require copying an already rendered compositor surface", async () => {
  let captures = 0;
  const result = await waitForPDFCanvas({ ...clock(), readState: async () => finished, captureFrame: async () => { captures++; throw new Error("UnknownVizError"); } });
  assert.equal(result, finished); assert.equal(captures, 0);
});

test("a transient Xvfb capture failure can recover only after actual canvas readiness", async () => {
  let reads = 0, captures = 0;
  const result = await waitForPDFCanvas({ ...clock(), readState: async () => ++reads < 4 ? pending : finished, captureFrame: async () => { if (++captures === 1) throw new Error("UnknownVizError"); } });
  assert.equal(result, finished); assert.equal(captures, 2); assert.equal(reads, 4);
});

test("persistent capture failure cannot hide a PDF that never renders and retries stay bounded", async () => {
  let captures = 0;
  await assert.rejects(waitForPDFCanvas({ ...clock(), timeoutMs: 1000, readState: async () => pending, captureFrame: async () => { captures++; throw new Error("UnknownVizError"); } }), error => /did not render/.test(error.message) && /"captureFailures":3/.test(error.message) && /UnknownVizError/.test(error.message));
  assert.equal(captures, 3);
});

test("PDF failure and unexpected capture errors remain fatal with useful context", async () => {
  await assert.rejects(waitForPDFCanvas({ ...clock(), readState: async () => ({ ...pending, note: "原页读取失败：文件损坏" }), captureFrame: async () => {} }), /文件损坏/);
  await assert.rejects(waitForPDFCanvas({ ...clock(), readState: async () => pending, captureFrame: async () => { throw new Error("Renderer crashed"); } }), /PDF compositor capture failed: Renderer crashed/);
});
