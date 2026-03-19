import { runtimeTranslate } from "./i18n";
import { isDesktopShell, writeDesktopClipboardText } from "./desktop-shell";

export async function writeClipboardText(value: string): Promise<void> {
  if (isDesktopShell()) {
    await writeDesktopClipboardText(value);
    return;
  }

  if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value);
    return;
  }

  if (typeof document !== "undefined") {
    const textarea = document.createElement("textarea");
    textarea.value = value;
    textarea.setAttribute("readonly", "true");
    textarea.setAttribute("aria-hidden", "true");
    textarea.style.position = "fixed";
    textarea.style.top = "-9999px";
    textarea.style.opacity = "0";
    document.body.appendChild(textarea);
    textarea.focus();
    textarea.select();
    textarea.setSelectionRange(0, textarea.value.length);

    try {
      if (
        typeof document.execCommand === "function" &&
        document.execCommand("copy")
      ) {
        return;
      }
    } finally {
      document.body.removeChild(textarea);
    }
  }

  throw new Error(runtimeTranslate("复制失败，请检查系统剪贴板权限"));
}
