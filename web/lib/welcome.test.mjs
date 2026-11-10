import { test } from "node:test";
import assert from "node:assert/strict";
import { carouselOrder, carouselWindow, personaTagline, pickUpCards } from "./welcome.ts";

const p = (id, chat_count) => ({ id, chat_count });

test("the carousel puts the default first, then the most used", () => {
  const order = carouselOrder([p("a", 1), p("b", 9), p("c", 4), p("d", 4)], "c");
  assert.deepEqual(order.map((x) => x.id), ["c", "b", "d", "a"]);
  assert.deepEqual(carouselOrder([p("a", 0), p("b", 0)], undefined).map((x) => x.id), ["a", "b"], "ties keep the server's order");
});

test("the carousel window wraps and never repeats anyone", () => {
  const five = ["a", "b", "c", "d", "e"];
  assert.deepEqual(carouselWindow(five, 0), { left: ["d", "e"], right: ["b", "c"] });
  assert.deepEqual(carouselWindow(five, 4), { left: ["c", "d"], right: ["a", "b"] });
  assert.deepEqual(carouselWindow(["a", "b"], 0), { left: [], right: ["b"] });
  assert.deepEqual(carouselWindow(["a", "b", "c"], 1), { left: ["a"], right: ["c"] });
  assert.deepEqual(carouselWindow(["a"], 0), { left: [], right: [] });
  const seven = carouselWindow(["a", "b", "c", "d", "e", "f", "g"], 3);
  assert.deepEqual(seven, { left: ["b", "c"], right: ["e", "f"] }, "at most two a side");
});

test("a persona's tagline is its tone and what it can do", () => {
  const label = (t) => t[0].toUpperCase() + t.slice(1);
  assert.equal(personaTagline({ tone: "friendly", capabilities: { code: true, images: true } }, label, false), "Friendly tone · writes code · makes images");
  assert.equal(personaTagline({ tone: "neutral", capabilities: {} }, label, true), "Neutral tone · your default");
  assert.match(personaTagline(undefined, label, true), /built-in assistant/);
});

const ago = (iso) => `ago(${iso.slice(11, 16)})`;
const chat = (id, at, extra = {}) => ({ id, title: `Chat ${id}`, message_count: 2, archived: false, created_at: at, last_message_at: at, persona: { name: "Maya", color: "#e0729b" }, ...extra });

test("pick-up cards: what needs you first, then unread replies, then recent chats, each place once", () => {
  const inbox = [
    { id: "i1", title: "WEB-42 needs your approval", href: "/inbox?item=i1", created_at: "2026-11-10T09:04:00Z", resolved: false, read: false, agent: { name: "Atlas" } },
    { id: "i2", title: "Done already", href: "/inbox?item=i2", created_at: "2026-11-10T09:00:00Z", resolved: true, read: false },
  ];
  const dock = [{ persona: { name: "Maya", color: "#e0729b" }, chats: [{ id: "c2", title: "Weekly spending check", state: "replied", unread: 1, last_line: "Was the $84 at REI a gift?" }] }];
  const chats = [chat("c1", "2026-11-10T08:00:00Z"), chat("c2", "2026-11-10T10:00:00Z"), chat("c3", "2026-11-09T08:00:00Z")];
  const cards = pickUpCards(inbox, dock, chats, ago);
  assert.deepEqual(cards.map((c) => [c.label, c.title, c.href]), [
    ["Needs you", "WEB-42 needs your approval", "/inbox?item=i1"],
    ["Replied", "Was the $84 at REI a gift?", "/chat/c2"],
    ["Continue", "Chat c1", "/chat/c1"],
  ]);
  assert.equal(cards[0].meta, "Atlas · ago(09:04)");
  assert.equal(cards[1].color, "#e0729b");
});

test("pick-up cards skip empty and archived chats and read replies", () => {
  const dock = [{ persona: { name: "Maya", color: "#e0729b" }, chats: [{ id: "c1", title: "t", state: "replied", unread: 0, last_line: "x" }, { id: "c4", title: "Asks", state: "needs_you", unread: 0, last_line: "" }] }];
  const chats = [chat("c1", "2026-11-10T08:00:00Z"), chat("c2", "2026-11-10T09:00:00Z", { message_count: 0 }), chat("c3", "2026-11-10T10:00:00Z", { archived: true })];
  assert.deepEqual(pickUpCards([], dock, chats, ago).map((c) => [c.label, c.href]), [["Needs you", "/chat/c4"], ["Continue", "/chat/c1"]]);
  assert.deepEqual(pickUpCards([], [], [], ago), []);
});
