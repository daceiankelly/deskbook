// Inline stroke icons (no icon font / CDN).
const base = {
  fill: 'none',
  stroke: 'currentColor',
  strokeWidth: 2,
  strokeLinecap: 'round',
  strokeLinejoin: 'round',
  viewBox: '0 0 24 24',
};

const make = (children) =>
  function Icon({ className = 'h-5 w-5', ...rest }) {
    return (
      <svg {...base} className={className} aria-hidden="true" {...rest}>
        {children}
      </svg>
    );
  };

export const CalendarIcon = make(
  <>
    <rect x="3" y="5" width="18" height="16" rx="2" />
    <path d="M16 3v4M8 3v4M3 10h18M8 14h2" />
  </>,
);
export const ChevronLeft = make(<path d="M15 6l-6 6 6 6" />);
export const ChevronRight = make(<path d="M9 6l6 6-6 6" />);
export const XIcon = make(<path d="M6 6l12 12M18 6L6 18" />);
export const FilterIcon = make(<path d="M4 7h16M7 12h10M10 17h4" />);
export const SearchIcon = make(
  <>
    <circle cx="11" cy="11" r="7" />
    <path d="M20 20l-3.5-3.5" />
  </>,
);
export const MapIcon = make(<path d="M4 6l5-2 6 2 5-2v14l-5 2-6-2-5 2zM9 4v14M15 6v14" />);
export const BookingsIcon = make(
  <>
    <rect x="4" y="4" width="16" height="17" rx="2" />
    <path d="M8 2v4M16 2v4M8 11h8M8 15h8" />
  </>,
);
export const UsersIcon = make(
  <>
    <circle cx="9" cy="8" r="3.5" />
    <path d="M2.5 20c.6-3.6 3.3-5.5 6.5-5.5s5.9 1.9 6.5 5.5M16 4.8a3.5 3.5 0 010 6.4M18 14.8c1.9.7 3.2 2.4 3.5 5.2" />
  </>,
);
export const SettingsIcon = make(
  <>
    <circle cx="12" cy="12" r="3" />
    <path d="M19.4 15a1.7 1.7 0 00.3 1.8l.1.1a2 2 0 11-2.8 2.8l-.1-.1a1.7 1.7 0 00-1.8-.3 1.7 1.7 0 00-1 1.5V21a2 2 0 11-4 0v-.1a1.7 1.7 0 00-1.1-1.5 1.7 1.7 0 00-1.8.3l-.1.1a2 2 0 11-2.8-2.8l.1-.1a1.7 1.7 0 00.3-1.8 1.7 1.7 0 00-1.5-1H3a2 2 0 110-4h.1a1.7 1.7 0 001.5-1.1 1.7 1.7 0 00-.3-1.8l-.1-.1a2 2 0 112.8-2.8l.1.1a1.7 1.7 0 001.8.3H9a1.7 1.7 0 001-1.5V3a2 2 0 114 0v.1a1.7 1.7 0 001 1.5 1.7 1.7 0 001.8-.3l.1-.1a2 2 0 112.8 2.8l-.1.1a1.7 1.7 0 00-.3 1.8V9a1.7 1.7 0 001.5 1H21a2 2 0 110 4h-.1a1.7 1.7 0 00-1.5 1z" />
  </>,
);
export const PlusIcon = make(<path d="M12 5v14M5 12h14" />);
export const MinusIcon = make(<path d="M5 12h14" />);
export const ResetIcon = make(<path d="M4 4v6h6M20 20v-6h-6M5.5 15a7 7 0 0012 2.5M18.5 9a7 7 0 00-12-2.5" />);
export const LogoutIcon = make(<path d="M15 4h4a1 1 0 011 1v14a1 1 0 01-1 1h-4M10 17l5-5-5-5M15 12H3" />);
export const RotateIcon = make(<path d="M20 11a8 8 0 10-2.3 5.7M20 4v7h-7" />);
export const TrashIcon = make(<path d="M4 7h16M10 11v6M14 11v6M5 7l1 13h12l1-13M9 7V4h6v3" />);
export const ClockIcon = make(
  <>
    <circle cx="12" cy="12" r="9" />
    <path d="M12 7v5l3 2" />
  </>,
);
