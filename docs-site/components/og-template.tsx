/**
 * Shared OG card, rendered by satori via `next/og`.
 *
 * Satori supports a narrow CSS subset: every element needs an explicit
 * `display`, colours must be literal (no CSS variables), and layout has to be
 * flexbox. Hex values below are the brand tokens resolved by hand:
 *   background hsl(224 12% 5%)  -> #0B0C0E
 *   cyan       #00ADB5 / #00C2CB
 *   orange     #FF5733
 */

const BG = '#0B0C0E';
const CYAN = '#00C2CB';
const ORANGE = '#FF5733';

export function OgTemplate({
  title,
  description,
  eyebrow = 'Ginboot',
}: {
  title: string;
  description?: string;
  eyebrow?: string;
}) {
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        width: '100%',
        height: '100%',
        backgroundColor: BG,
        backgroundImage: `radial-gradient(ellipse 70% 60% at 8% 4%, rgba(0, 194, 203, 0.22) 0%, transparent 62%), radial-gradient(ellipse 60% 55% at 96% 96%, rgba(255, 87, 51, 0.16) 0%, transparent 60%)`,
        padding: 72,
        justifyContent: 'space-between',
      }}
    >
      {/* Brand row */}
      <div style={{ display: 'flex', alignItems: 'center' }}>
        <svg width="52" height="52" viewBox="0 0 32 32">
          <defs>
            <linearGradient id="og-tile" x1="0" y1="0" x2="32" y2="32" gradientUnits="userSpaceOnUse">
              <stop offset="0" stopColor="#00C2CB" />
              <stop offset="1" stopColor="#00858C" />
            </linearGradient>
          </defs>
          <rect width="32" height="32" rx="7" fill="url(#og-tile)" />
          <path
            d="M17.9 6.6 10.4 18.2h4.9l-1.9 7.2 7.9-11.6h-4.9l1.5-7.2Z"
            fill={ORANGE}
            stroke="#00858C"
            strokeWidth="1.2"
            strokeLinejoin="round"
          />
        </svg>
        <div
          style={{
            display: 'flex',
            marginLeft: 18,
            fontSize: 30,
            fontWeight: 600,
            color: '#F2F4F5',
            letterSpacing: -0.5,
          }}
        >
          {eyebrow}
        </div>
      </div>

      {/* Title block */}
      <div style={{ display: 'flex', flexDirection: 'column' }}>
        <div
          style={{
            display: 'flex',
            fontSize: 68,
            fontWeight: 700,
            color: '#FFFFFF',
            letterSpacing: -2,
            lineHeight: 1.1,
            // satori has no line-clamp; cap the box instead
            maxHeight: 230,
            overflow: 'hidden',
          }}
        >
          {title}
        </div>
        {description ? (
          <div
            style={{
              display: 'flex',
              marginTop: 24,
              fontSize: 30,
              color: '#9BA3A8',
              lineHeight: 1.4,
              maxHeight: 130,
              overflow: 'hidden',
            }}
          >
            {description}
          </div>
        ) : null}
      </div>

      {/* Accent rule */}
      <div
        style={{
          display: 'flex',
          height: 8,
          width: '100%',
          borderRadius: 4,
          backgroundImage: `linear-gradient(to right, ${CYAN}, ${ORANGE})`,
        }}
      />
    </div>
  );
}
