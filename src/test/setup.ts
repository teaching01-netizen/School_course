import "@testing-library/jest-dom";

Object.defineProperty(window, "scrollTo", {
  configurable: true,
  value: () => undefined,
  writable: true,
});

// jsdom does not expose localStorage for opaque origins; the schedule-impact
// queue persists its density preference there.
const storageStore = new Map<string, string>();
const localStorageStub: Storage = {
  get length() { return storageStore.size; },
  clear: () => storageStore.clear(),
  getItem: (key: string) => storageStore.get(key) ?? null,
  key: (index: number) => [...storageStore.keys()][index] ?? null,
  removeItem: (key: string) => { storageStore.delete(key); },
  setItem: (key: string, value: string) => { storageStore.set(key, String(value)); },
};
Object.defineProperty(window, "localStorage", {
  configurable: true,
  value: localStorageStub,
});

if (!HTMLDialogElement.prototype.showModal) {
  HTMLDialogElement.prototype.showModal = function () {
    if (this.hasAttribute("open")) throw new Error("Dialog is already open");
    this.setAttribute("open", "");
    this.dispatchEvent(new Event("open"));
  };
  HTMLDialogElement.prototype.close = function () {
    if (!this.hasAttribute("open")) return;
    this.removeAttribute("open");
    this.dispatchEvent(new Event("close"));
  };
  Object.defineProperty(HTMLDialogElement.prototype, "closedBy", {
    get: () => "any",
    configurable: true,
  });
}
