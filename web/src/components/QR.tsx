import { renderSVG } from "uqr";
import { useMemo } from "react";

/** A QR code for a phone's camera, black on white in either theme so every camera reads it. */
export function QR({ value, size = 168 }: { value: string; size?: number }) {
  const svg = useMemo(() => renderSVG(value, { border: 2 }), [value]);
  return (
    <div
      role="img"
      aria-label={`QR code for ${value}`}
      className="shrink-0 overflow-hidden rounded-xl bg-white p-1 shadow-[0_0_0_0.5px_var(--border)] [&>svg]:h-full [&>svg]:w-full"
      style={{ width: size, height: size }}
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  );
}
