import { builtinSoundSrc, fetchCustomAlertSoundUrl } from "./userPreferences";

let unlocked = false;
let lastPlayAt = 0;
const MIN_GAP_MS = 1200;
const objectUrls = new Map<string, string>();

export function unlockAlertAudio() {
  if (unlocked) return;
  unlocked = true;
  try {
    const a = new Audio();
    a.muted = true;
    void a.play().then(() => {
      a.pause();
    }).catch(() => {
      unlocked = false;
    });
  } catch {
    unlocked = false;
  }
}

export function armAlertAudioUnlock() {
  const once = () => {
    unlockAlertAudio();
    window.removeEventListener("pointerdown", once);
    window.removeEventListener("keydown", once);
  };
  window.addEventListener("pointerdown", once, { once: true });
  window.addEventListener("keydown", once, { once: true });
}

export async function playAlertSound(soundId: string): Promise<void> {
  const now = Date.now();
  if (now - lastPlayAt < MIN_GAP_MS) return;
  lastPlayAt = now;
  unlockAlertAudio();
  const builtin = builtinSoundSrc(soundId);
  let src = builtin;
  if (!src && soundId.startsWith("custom:")) {
    let cached = objectUrls.get(soundId);
    if (!cached) {
      cached = await fetchCustomAlertSoundUrl(soundId);
      objectUrls.set(soundId, cached);
    }
    src = cached;
  }
  if (!src) src = builtinSoundSrc("builtin:alert") ?? "/sounds/alert.wav";
  const audio = new Audio(src);
  audio.volume = 0.7;
  try {
    await audio.play();
  } catch {
    /* autoplay bloqueado até gesto do usuário */
  }
}
