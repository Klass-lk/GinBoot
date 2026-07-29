import { cn } from '@/lib/cn';
import type { ComponentProps } from 'react';

/**
 * The tile mark on its own. Gradient matches ginboot-cloud's favicon.svg so the
 * framework and the platform read as one product; the bolt is the shared
 * secondary brand hue.
 *
 * `id`s are suffixed per-instance because the mark renders more than once per
 * page (navbar + footer) and duplicate gradient ids collide in Safari.
 */
export function LogoMark({
  className,
  id = 'default',
  ...props
}: ComponentProps<'svg'> & { id?: string }) {
  const gradientId = `gb-tile-${id}`;

  return (
    <svg
      viewBox="0 0 32 32"
      role="img"
      aria-label="Ginboot"
      className={cn('size-7 shrink-0', className)}
      {...props}
    >
      <defs>
        <linearGradient id={gradientId} x1="0" y1="0" x2="32" y2="32" gradientUnits="userSpaceOnUse">
          <stop offset="0" stopColor="#00C2CB" />
          <stop offset="1" stopColor="#00858C" />
        </linearGradient>
      </defs>
      <rect width="32" height="32" rx="7" fill={`url(#${gradientId})`} />
      {/* Bolt outlined in the deep end of the tile gradient so it stays legible at 16px */}
      <path
        d="M17.9 6.6 10.4 18.2h4.9l-1.9 7.2 7.9-11.6h-4.9l1.5-7.2Z"
        fill="#FF5733"
        stroke="#00858C"
        strokeWidth="1.2"
        strokeLinejoin="round"
      />
    </svg>
  );
}

/** Mark + wordmark, for the navbar and footer. */
export function Logo({ className, ...props }: ComponentProps<'div'>) {
  return (
    <div className={cn('flex items-center gap-2', className)} {...props}>
      <LogoMark id="wordmark" className="size-6" />
      <span className="text-[15px] font-semibold tracking-tight">ginboot</span>
    </div>
  );
}
