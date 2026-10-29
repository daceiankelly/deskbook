import { useCallback, useEffect, useState } from 'react';
import { api } from '../api.js';
import FloorPlan, { PLAN_H, PLAN_W } from './FloorPlan.jsx';
import { PlusIcon, RotateIcon, TrashIcon } from './icons.jsx';

function Notice({ msg }) {
  if (!msg) return null;
  return (
    <div role="status" className={`mb-4 rounded-md p-3 text-sm ${msg.ok ? 'bg-green-50 text-green-800' : 'bg-red-50 text-red-800'}`}>
      {msg.text}
    </div>
  );
}

/* ---------------- floor plan & desks ---------------- */

function DeskEditor({ desks, onDesksChanged }) {
  const [selectedId, setSelectedId] = useState(null);
  const [form, setForm] = useState(null);
  const [msg, setMsg] = useState(null);
  const desk = desks.find((d) => d.id === selectedId);

  useEffect(() => {
    setForm(desk ? { label: desk.label, zone: desk.zone, x: desk.x, y: desk.y, rotation: desk.rotation, active: desk.active } : null);
  }, [desk]);

  const run = async (fn, okText) => {
    try {
      await fn();
      await onDesksChanged();
      setMsg(okText ? { ok: true, text: okText } : null);
    } catch (e) {
      setMsg({ ok: false, text: e.message });
    }
  };

  const save = () =>
    run(() => api.admin.updateDesk(desk.id, { ...form, x: Number(form.x), y: Number(form.y) }), `Saved ${form.label}`);

  return (
    <div className="flex h-full gap-4">
      <div className="card relative min-w-0 flex-1 overflow-hidden">
        <FloorPlan
          desks={desks}
          mode="edit"
          selectedDeskId={selectedId}
          onDeskClick={setSelectedId}
          onDeskMove={(id, x, y) => run(() => api.admin.updateDesk(id, { x, y }))}
        />
      </div>
      <div className="w-80 shrink-0 space-y-4 overflow-y-auto">
        <Notice msg={msg} />
        <p className="text-xs text-ink-muted">Drag desks on the plan to reposition them. Changes save when you drop.</p>
        <button
          className="btn-primary w-full"
          onClick={() =>
            run(async () => {
              const { desk: d } = await api.admin.createDesk({ label: `New ${desks.length + 1}`, x: PLAN_W / 2, y: PLAN_H / 2, rotation: 0 });
              setSelectedId(d.id);
            }, 'Desk added in the centre of the plan')
          }
        >
          <PlusIcon className="h-4 w-4" /> Add desk
        </button>
        {!form ? (
          <p className="card p-4 text-sm text-ink-muted">Select a desk on the plan to edit it. {desks.length} desks configured.</p>
        ) : (
          <div className="card space-y-3 p-4">
            <div className="text-xs text-ink-muted">ID {desk.id}</div>
            <div>
              <label className="label" htmlFor="d-label">Label</label>
              <input id="d-label" className="input" value={form.label} onChange={(e) => setForm({ ...form, label: e.target.value })} />
            </div>
            <div>
              <label className="label" htmlFor="d-zone">Zone</label>
              <input id="d-zone" className="input" value={form.zone} onChange={(e) => setForm({ ...form, zone: e.target.value })} />
            </div>
            <div className="grid grid-cols-2 gap-2">
              <div>
                <label className="label" htmlFor="d-x">X</label>
                <input id="d-x" type="number" className="input" value={form.x} onChange={(e) => setForm({ ...form, x: e.target.value })} />
              </div>
              <div>
                <label className="label" htmlFor="d-y">Y</label>
                <input id="d-y" type="number" className="input" value={form.y} onChange={(e) => setForm({ ...form, y: e.target.value })} />
              </div>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm">Rotation {form.rotation}°</span>
              <button className="btn-ghost px-3 py-1.5" onClick={() => setForm({ ...form, rotation: (form.rotation + 90) % 360 })}>
                <RotateIcon className="h-4 w-4" /> Rotate
              </button>
            </div>
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={form.active} onChange={(e) => setForm({ ...form, active: e.target.checked })} />
              Bookable (inactive desks show as unavailable)
            </label>
            <div className="flex gap-2 pt-2">
              <button className="btn-primary flex-1" onClick={save}>Save</button>
              <button
                className="btn-danger"
                aria-label="Delete desk"
                onClick={() =>
                  window.confirm(`Delete desk ${desk.label}? Desks with booking history must be deactivated instead.`) &&
                  run(async () => {
                    await api.admin.deleteDesk(desk.id);
                    setSelectedId(null);
                  }, 'Desk deleted')
                }
              >
                <TrashIcon className="h-4 w-4" />
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

/* ---------------- employees ---------------- */

function Employees({ me }) {
  const [list, setList] = useState([]);
  const [msg, setMsg] = useState(null);
  const empty = { username: '', name: '', initials: '', role: 'employee', password: '' };
  const [form, setForm] = useState(empty);

  const load = useCallback(async () => {
    try {
      setList((await api.admin.employees()).employees);
    } catch (e) {
      setMsg({ ok: false, text: e.message });
    }
  }, []);
  useEffect(() => {
    load();
  }, [load]);

  const update = async (id, patch, okText) => {
    try {
      await api.admin.updateEmployee(id, patch);
      await load();
      setMsg({ ok: true, text: okText });
    } catch (e) {
      setMsg({ ok: false, text: e.message });
    }
  };

  const create = async (e) => {
    e.preventDefault();
    try {
      await api.admin.createEmployee(form);
      setForm(empty);
      await load();
      setMsg({ ok: true, text: `Created ${form.username}` });
    } catch (err) {
      setMsg({ ok: false, text: err.message });
    }
  };

  return (
    <div className="grid gap-6 xl:grid-cols-[1fr_320px]">
      <div>
        <Notice msg={msg} />
        <table className="card w-full overflow-hidden text-sm">
          <thead className="bg-slate-50 text-left text-xs uppercase text-ink-muted">
            <tr>
              <th className="px-4 py-2">Name</th>
              <th className="px-4 py-2">Username</th>
              <th className="px-4 py-2">Role</th>
              <th className="px-4 py-2">Status</th>
              <th className="px-4 py-2" />
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {list.map((u) => (
              <tr key={u.id} className={u.active ? '' : 'text-ink-muted'}>
                <td className="px-4 py-2 font-medium">{u.name}</td>
                <td className="px-4 py-2">{u.username}</td>
                <td className="px-4 py-2">
                  <select
                    className="rounded border border-slate-200 px-2 py-1"
                    value={u.role}
                    disabled={u.id === me.id}
                    onChange={(e) => update(u.id, { role: e.target.value }, `${u.name} is now ${e.target.value}`)}
                  >
                    <option value="employee">employee</option>
                    <option value="admin">admin</option>
                  </select>
                </td>
                <td className="px-4 py-2">{u.active ? 'Active' : 'Deactivated'}</td>
                <td className="space-x-3 whitespace-nowrap px-4 py-2 text-right">
                  <button
                    className="text-xs font-semibold text-brand hover:underline"
                    onClick={() => {
                      const pw = window.prompt(`New password for ${u.name} (min 10 characters)`);
                      if (pw) update(u.id, { password: pw }, `Password reset for ${u.name}; their sessions were signed out`);
                    }}
                  >
                    Reset password
                  </button>
                  {u.id !== me.id && (
                    <button
                      className={`text-xs font-semibold hover:underline ${u.active ? 'text-red-600' : 'text-green-700'}`}
                      onClick={() =>
                        (!u.active || window.confirm(`Deactivate ${u.name}? Their upcoming bookings will be cancelled.`)) &&
                        update(u.id, { active: !u.active }, `${u.name} ${u.active ? 'deactivated' : 'reactivated'}`)
                      }
                    >
                      {u.active ? 'Deactivate' : 'Reactivate'}
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <form className="card h-fit space-y-3 p-4" onSubmit={create}>
        <h3 className="font-semibold">Add employee</h3>
        {[
          ['name', 'Full name', 'text'],
          ['username', 'Username', 'text'],
          ['initials', 'Initials (optional)', 'text'],
          ['password', 'Initial password', 'password'],
        ].map(([k, label, type]) => (
          <div key={k}>
            <label className="label" htmlFor={`emp-${k}`}>{label}</label>
            <input id={`emp-${k}`} type={type} className="input" value={form[k]} onChange={(e) => setForm({ ...form, [k]: e.target.value })} required={k !== 'initials'} autoComplete="off" />
          </div>
        ))}
        <div>
          <label className="label" htmlFor="emp-role">Role</label>
          <select id="emp-role" className="input" value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value })}>
            <option value="employee">Employee</option>
            <option value="admin">Administrator</option>
          </select>
        </div>
        <button className="btn-primary w-full" type="submit">Create</button>
      </form>
    </div>
  );
}

/* ---------------- settings ---------------- */

function Settings({ settings, onSaved }) {
  const [form, setForm] = useState(settings);
  const [msg, setMsg] = useState(null);
  const num = ['windowDays', 'openHour', 'closeHour', 'backupRetention'];
  const fields = [
    ['officeName', 'Office name'],
    ['floorName', 'Floor name'],
    ['timezone', 'Office timezone (IANA, e.g. Europe/Amsterdam)'],
    ['windowDays', 'Booking window (days)'],
    ['openHour', 'Opening hour (0-23)'],
    ['closeHour', 'Closing hour (1-24)'],
    ['backupRetention', 'Backups to keep'],
  ];
  const save = async (e) => {
    e.preventDefault();
    try {
      const body = { ...form };
      num.forEach((k) => (body[k] = Number(body[k])));
      await api.admin.updateSettings(body);
      setMsg({ ok: true, text: 'Settings saved' });
      onSaved();
    } catch (err) {
      setMsg({ ok: false, text: err.message });
    }
  };
  return (
    <form className="card max-w-xl space-y-3 p-5" onSubmit={save}>
      <Notice msg={msg} />
      {fields.map(([k, label]) => (
        <div key={k}>
          <label className="label" htmlFor={`set-${k}`}>{label}</label>
          <input id={`set-${k}`} className="input" type={num.includes(k) ? 'number' : 'text'} value={form[k]} onChange={(e) => setForm({ ...form, [k]: e.target.value })} required />
        </div>
      ))}
      <p className="text-xs text-ink-muted">
        Changing the timezone or opening hours affects new bookings only; existing bookings keep their stored office-local date and hours.
      </p>
      <button className="btn-primary" type="submit">Save settings</button>
    </form>
  );
}

/* ---------------- backups ---------------- */

function Backups() {
  const [list, setList] = useState([]);
  const [msg, setMsg] = useState(null);
  const load = useCallback(async () => {
    try {
      setList((await api.admin.backups()).backups);
    } catch (e) {
      setMsg({ ok: false, text: e.message });
    }
  }, []);
  useEffect(() => {
    load();
  }, [load]);
  return (
    <div className="max-w-3xl">
      <Notice msg={msg} />
      <div className="mb-4 flex items-center justify-between">
        <p className="text-sm text-ink-muted">A backup is written automatically after every change. Restoring requires stopping the server (see docs/backup-recovery.md).</p>
        <button
          className="btn-primary shrink-0"
          onClick={async () => {
            try {
              const { name } = await api.admin.createBackup();
              setMsg({ ok: true, text: `Created ${name}` });
              load();
            } catch (e) {
              setMsg({ ok: false, text: e.message });
            }
          }}
        >
          Back up now
        </button>
      </div>
      <ul className="card divide-y divide-slate-100 text-sm">
        {list.map((b) => (
          <li key={b.name} className="flex justify-between px-4 py-2">
            <span className="font-mono text-xs">{b.name}</span>
            <span className="text-ink-muted">rev {b.revision} · {(b.size / 1024).toFixed(1)} KB</span>
          </li>
        ))}
        {list.length === 0 && <li className="px-4 py-3 text-ink-muted">No backups yet.</li>}
      </ul>
    </div>
  );
}

const TABS = [
  ['plan', 'Floor plan & desks'],
  ['employees', 'Employees'],
  ['settings', 'Settings'],
  ['backups', 'Backups'],
];

export default function AdminPage({ me, desks, settings, onDesksChanged, onSettingsChanged }) {
  const [tab, setTab] = useState('plan');
  return (
    <div className="flex h-full flex-col px-8 py-6">
      <h1 className="text-2xl font-bold">Administration</h1>
      <div className="mb-5 mt-3 flex gap-1 border-b border-slate-200">
        {TABS.map(([k, label]) => (
          <button
            key={k}
            onClick={() => setTab(k)}
            className={`-mb-px border-b-2 px-4 py-2 text-sm font-semibold ${tab === k ? 'border-brand text-brand' : 'border-transparent text-ink-muted hover:text-ink'}`}
          >
            {label}
          </button>
        ))}
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">
        {tab === 'plan' && <DeskEditor desks={desks} onDesksChanged={onDesksChanged} />}
        {tab === 'employees' && <Employees me={me} />}
        {tab === 'settings' && <Settings settings={settings} onSaved={onSettingsChanged} />}
        {tab === 'backups' && <Backups />}
      </div>
    </div>
  );
}
