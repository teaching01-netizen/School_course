const updateAriaState = (event: Event) => {
  const input = event.target;
  if (!(input instanceof HTMLElement) || !input.matches?.('input, textarea, select')) return;
  const isUserInvalid = input.matches(':user-invalid');
  if (isUserInvalid) {
    input.setAttribute('aria-invalid', 'true');
  } else {
    input.removeAttribute('aria-invalid');
  }
};

export function initUserInvalidSync() {
  document.addEventListener('blur', updateAriaState, true);
  document.addEventListener('focus', updateAriaState, true);
  document.addEventListener('input', (event) => {
    const input = event.target;
    if (!(input instanceof HTMLElement)) return;
    if (input.hasAttribute('aria-invalid')) updateAriaState(event);
  });
}
