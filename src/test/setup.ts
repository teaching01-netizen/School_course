import "@testing-library/jest-dom";

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
