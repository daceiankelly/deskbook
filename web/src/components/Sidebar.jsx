import { useState } from 'react';
import { BookingsIcon, LogoutIcon, MapIcon, SettingsIcon, UsersIcon } from './icons.jsx';

function NavItem({ active, icon: Icon, label, onClick }) {
  return (
    <button
      onClick={onClick}
      className={`relative flex w-full flex-col items-center gap-1 py-3 text-[11px] font-bold uppercase tracking-wide ${
        active ? 'text-brand' : 'text-ink hover:text-brand'
      }`}
      aria-current={active ? 'page' : undefined}
    >
      <span className={`flex h-12 w-12 items-center justify-center rounded-lg ${active ? 'bg-brand-pale' : ''}`}>
        <Icon className="h-6 w-6" />
      </span>
      {label}
      {active && <span className="absolute right-0 top-2 h-[calc(100%-16px)] w-[3px] rounded-l bg-brand" />}
    </button>
  );
}

export default function Sidebar({ view, onNavigate, user, onLogout }) {
  const [menu, setMenu] = useState(false);
  return (
    <nav className="z-20 flex w-24 shrink-0 flex-col items-center border-r border-slate-100 bg-white py-5 shadow-[2px_0_8px_rgba(15,23,42,0.04)]">
      <div className="mb-8 flex h-14 w-14 items-center justify-center rounded-lg bg-gradient-to-br from-brand to-sky-500 text-white shadow-card" title="Musical Chairs">
        <svg viewBox="0 0 32 32" className="h-9 w-9" aria-label="Musical Chairs">
          <path d="M9 22c0-7 3-12 10-12h4" stroke="#fff" strokeWidth="4" fill="none" strokeLinecap="round" />
          <path d="M8 17h10" stroke="#fff" strokeWidth="4" strokeLinecap="round" />
        </svg>
      </div>
      <NavItem active={view === 'map'} icon={MapIcon} label="Map" onClick={() => onNavigate('map')} />
      <div className="my-2 h-px w-16 bg-slate-200" />
      <NavItem active={view === 'bookings'} icon={BookingsIcon} label="Bookings" onClick={() => onNavigate('bookings')} />
      <NavItem active={view === 'users'} icon={UsersIcon} label="Users" onClick={() => onNavigate('users')} />
      {user.role === 'admin' && (
        <NavItem active={view === 'admin'} icon={SettingsIcon} label="Admin" onClick={() => onNavigate('admin')} />
      )}
      <div className="relative mt-auto">
        <button
          onClick={() => setMenu((m) => !m)}
          className="relative flex h-12 w-12 items-center justify-center rounded-full border-2 border-brand/30 bg-white text-sm font-bold text-brand"
          title={user.name}
        >
          {user.initials || user.name.slice(0, 2).toUpperCase()}
          <span className="absolute -bottom-0.5 -right-0.5 h-3.5 w-3.5 rounded-full border-2 border-white bg-desk-available" />
        </button>
        {menu && (
          <div className="absolute bottom-0 left-14 w-48 rounded-lg bg-white p-2 shadow-card ring-1 ring-slate-100">
            <div className="px-2 py-1 text-sm font-semibold">{user.name}</div>
            <div className="px-2 pb-2 text-xs text-ink-muted">
              {user.username} · {user.role}
            </div>
            <button onClick={onLogout} className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-slate-50">
              <LogoutIcon className="h-4 w-4" /> Sign out
            </button>
          </div>
        )}
      </div>
    </nav>
  );
}
