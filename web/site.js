// Hero wipe: drag or use the arrow keys to move the cut between pixels and SVG.
(() => {
  const wipe = document.getElementById('wipe');
  if (!wipe) return;
  const frame = wipe.querySelector('.frame');
  const range = wipe.querySelector('input[type=range]');
  const set = (pct) => {
    pct = Math.max(0, Math.min(100, pct));
    wipe.style.setProperty('--cut', pct + '%');
    range.value = String(Math.round(pct));
    range.setAttribute('aria-valuenow', range.value);
  };
  const fromEvent = (e) => {
    const r = frame.getBoundingClientRect();
    set(((e.clientX - r.left) / r.width) * 100);
  };
  let down = false;
  frame.addEventListener('pointerdown', (e) => { down = true; frame.setPointerCapture(e.pointerId); fromEvent(e); });
  frame.addEventListener('pointermove', (e) => { if (down) fromEvent(e); });
  frame.addEventListener('pointerup', () => { down = false; });
  frame.addEventListener('pointercancel', () => { down = false; });
  range.addEventListener('input', () => set(Number(range.value)));
  set(50);
})();
