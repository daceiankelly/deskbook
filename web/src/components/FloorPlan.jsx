import { useCallback, useEffect, useRef, useState } from 'react';
import { MinusIcon, PlusIcon, ResetIcon } from './icons.jsx';

// Plan coordinates match the desk x/y stored on the server.
export const PLAN_W = 1260;
export const PLAN_H = 880;

const C = {
  wall: '#2E7DC4',
  floor: '#EAF4FC',
  available: '#66CC1F',
  booked: '#FDB813',
  selected: '#1E6FD9',
  unavailable: '#C5CCD3',
  green: '#1A9A47',
  red: '#E5322D',
  you: '#1E6FD9',
};

const OUTLINE = '17,13 525,13 1245,72 1210,372 530,318 530,858 10,858 10,315 17,315';

/* ---------- static building layer (from the design reference) ---------- */

function MeetingIcon({ x, y, s = 1 }) {
  return (
    <g transform={`translate(${x} ${y}) scale(${s})`} fill={C.available}>
      <circle cx="0" cy="-26" r="6" />
      <circle cx="-26" cy="-4" r="5.5" />
      <circle cx="26" cy="-4" r="5.5" />
      <circle cx="-14" cy="-16" r="5.5" />
      <circle cx="14" cy="-16" r="5.5" />
      <path d="M-22 18 L-12 -4 L12 -4 L22 18 Z" />
      <path d="M-30 6 a8 8 0 0 1 10 -4 L-24 16 Z M30 6 a8 8 0 0 0 -10 -4 L24 16 Z" />
    </g>
  );
}

function FirstAid({ x, y }) {
  return (
    <g transform={`translate(${x} ${y})`}>
      <rect x="-20" y="-20" width="40" height="40" fill={C.green} />
      <path d="M-5 -14h10v9h9v10h-9v9h-10v-9h-9v-10h9z" fill="#fff" />
    </g>
  );
}

function Extinguisher({ x, y }) {
  return (
    <g transform={`translate(${x} ${y})`}>
      <rect x="-21" y="-21" width="42" height="42" fill={C.red} />
      <rect x="-13" y="-8" width="10" height="24" rx="3" fill="#fff" />
      <path d="M-11 -9 v-5 h6 v5 M-5 -14 h8 l3 4" stroke="#fff" strokeWidth="2" fill="none" />
      <path d="M5 -2 q6 4 0 8 q6 4 0 8 M11 -2 q6 4 0 8 q6 4 0 8" stroke="#fff" strokeWidth="2" fill="none" />
    </g>
  );
}

function ExitSign({ x, y, dir = 'up' }) {
  const arrow =
    dir === 'up' ? <path d="M18 14 v-16 m-8 8 l8 -9 l8 9" /> : <path d="M-6 0 h-22 m9 -8 l-9 8 l9 8" />;
  return (
    <g transform={`translate(${x} ${y})`}>
      <rect x="-38" y="-18" width="76" height="36" fill={C.green} />
      <g stroke="#fff" strokeWidth="4" fill="none" strokeLinecap="round" strokeLinejoin="round">
        {arrow}
        <rect x={dir === 'up' ? -30 : 2} y="-12" width="22" height="24" strokeWidth="2.5" />
        <path d={dir === 'up' ? 'M-22 -6 l4 4 l-4 4 l4 6 M-18 -2 l5 -2' : 'M10 -6 l4 4 l-4 4 l4 6 M14 -2 l5 -2'} strokeWidth="2.5" />
      </g>
    </g>
  );
}

function Building({ showSafety }) {
  const line = { stroke: C.wall, strokeWidth: 1.5, fill: 'none' };
  return (
    <g>
      <polygon points={OUTLINE} fill={C.floor} stroke={C.wall} strokeWidth="5" strokeLinejoin="miter" />
      {/* kitchen / entrance area */}
      <path d="M20 45 h80 v-32" {...line} />
      <rect x="20" y="85" width="122" height="30" {...line} />
      <rect x="74" y="94" width="12" height="8" fill="#F59E0B" rx="2" />
      <path d="M356 13 v20 h100 v-20 M346 78 v22 h120 v-22" {...line} />
      {/* stairs */}
      <rect x="20" y="216" width="200" height="76" {...line} />
      <path d="M140 216 v76" {...line} />
      {Array.from({ length: 10 }, (_, i) => (
        <path key={i} d={`M${60 + i * 6} 218 v72`} stroke={C.wall} strokeWidth="1" />
      ))}
      <path d="M58 254 h66" {...line} />
      {/* columns & benches along the corridor */}
      {[225, 313, 515].map((x) => (
        <rect key={x} x={x - 11} y="292" width="22" height="22" {...line} />
      ))}
      <rect x="344" y="292" width="138" height="20" {...line} />
      <path d="M413 292 v20" {...line} />
      {/* lower wing: meeting room */}
      <rect x="10" y="292" width="208" height="195" {...line} />
      <path d="M218 385 l8 -12 M218 385 l0 22" {...line} />
      {/* screens between clusters */}
      <rect x="372" y="397" width="20" height="68" {...line} />
      <rect x="372" y="560" width="20" height="68" {...line} />
      <rect x="372" y="725" width="20" height="68" {...line} />
      <rect x="153" y="552" width="17" height="88" {...line} />
      <rect x="153" y="715" width="17" height="88" {...line} />
      <path d="M404 514 h96 M40 676 h96 M404 676 h96" {...line} />
      {/* small room bottom */}
      <rect x="233" y="795" width="74" height="57" {...line} />
      {/* upper wing: three meeting pods and right room */}
      {[
        [520, 28],
        [665, 34],
        [796, 44],
      ].map(([x, y]) => (
        <g key={x}>
          <rect x={x} y={y} width="76" height="58" {...line} transform={`rotate(4 ${x} ${y})`} />
          <rect x={x + 4} y={y + 4} width="68" height="50" {...line} transform={`rotate(4 ${x} ${y})`} />
        </g>
      ))}
      <path d="M1115 60 L1085 358" {...line} />
      <path d="M1100 210 l8 -14" {...line} />
      {/* monitors above upper clusters */}
      <rect x="553" y="162" width="70" height="20" {...line} />
      <rect x="744" y="170" width="69" height="22" {...line} />
      <rect x="948" y="188" width="70" height="22" {...line} />

      <MeetingIcon x={114} y={388} s={1.1} />
      <MeetingIcon x={273} y={826} s={0.8} />
      <MeetingIcon x={558} y={60} s={0.8} />
      <MeetingIcon x={702} y={65} s={0.8} />
      <MeetingIcon x={833} y={75} s={0.8} />
      <MeetingIcon x={1167} y={205} s={1} />

      <text x="470" y="828" fontSize="16" fill={C.wall} textAnchor="middle">
        Support
      </text>

      {showSafety && (
        <g>
          <FirstAid x={120} y={43} />
          <Extinguisher x={163} y={43} />
          <ExitSign x={265} y={43} dir="up" />
          <ExitSign x={265} y={257} dir="left" />
          <Extinguisher x={678} y={292} />
          <Extinguisher x={335} y={828} />
        </g>
      )}
    </g>
  );
}

/* ---------- desks ---------- */

export function deskColor(state) {
  return C[state] || C.unavailable;
}

function DeskGlyph({ desk, x, y, fill, outlined, interactive, onPointerDown, onClick, title }) {
  return (
    <g
      transform={`translate(${x} ${y}) rotate(${desk.rotation || 0})`}
      className={interactive ? 'cursor-pointer' : 'cursor-not-allowed'}
      onPointerDown={onPointerDown}
      onClick={onClick}
      role="button"
      tabIndex={interactive ? 0 : -1}
      aria-label={title}
      onKeyDown={(e) => {
        if (interactive && (e.key === 'Enter' || e.key === ' ')) {
          e.preventDefault();
          onClick?.();
        }
      }}
    >
      <title>{title}</title>
      <circle className="desk-glyph" cx="0" cy="-12" r="12" fill={fill} />
      <rect
        className="desk-glyph"
        x="-27"
        y="-10"
        width="54"
        height="22"
        rx="4"
        fill={fill}
        stroke={outlined ? '#0B4FA8' : '#fff'}
        strokeWidth={outlined ? 3 : 1.5}
      />
    </g>
  );
}

function Bubble({ x, y, text, color = C.you }) {
  const w = Math.max(46, text.length * 9 + 18);
  return (
    <g transform={`translate(${x} ${y - 34})`} pointerEvents="none">
      <rect x={-w / 2} y="-24" width={w} height="26" rx="13" fill={color} stroke="#fff" strokeWidth="2" />
      <path d="M-6 1 L0 9 L6 1 Z" fill={color} />
      <text y="-6" textAnchor="middle" fontSize="15" fontWeight="600" fill="#fff">
        {text}
      </text>
    </g>
  );
}

/**
 * Interactive SVG floor plan.
 *
 * mode="view": desks coloured by `states[deskId].state`; click calls onDeskClick.
 * mode="edit": desks draggable; onDeskMove(id, x, y) fires on drop.
 */
export default function FloorPlan({
  desks,
  states = {},
  mode = 'view',
  selectedDeskId,
  highlight = [], // [{deskId, label}]
  onDeskClick,
  onDeskMove,
  showSafety = true,
  extraControls = null, // rendered above the zoom buttons
}) {
  const svgRef = useRef(null);
  const [view, setView] = useState({ scale: 1, tx: 0, ty: 0 });
  const [drag, setDrag] = useState(null); // {kind:'pan'|'desk', ...}
  const [dragPos, setDragPos] = useState(null); // {id, x, y}
  const viewRef = useRef(view);
  viewRef.current = view;

  const toSvg = useCallback((e) => {
    const svg = svgRef.current;
    const pt = svg.createSVGPoint();
    pt.x = e.clientX;
    pt.y = e.clientY;
    return pt.matrixTransform(svg.getScreenCTM().inverse());
  }, []);

  const zoomAt = useCallback((factor, center) => {
    setView((v) => {
      const scale = Math.min(4, Math.max(0.5, v.scale * factor));
      const k = scale / v.scale;
      const cx = center?.x ?? PLAN_W / 2;
      const cy = center?.y ?? PLAN_H / 2;
      return { scale, tx: cx - (cx - v.tx) * k, ty: cy - (cy - v.ty) * k };
    });
  }, []);

  useEffect(() => {
    const svg = svgRef.current;
    const onWheel = (e) => {
      e.preventDefault();
      zoomAt(e.deltaY < 0 ? 1.15 : 1 / 1.15, toSvg(e));
    };
    svg.addEventListener('wheel', onWheel, { passive: false });
    return () => svg.removeEventListener('wheel', onWheel);
  }, [toSvg, zoomAt]);

  const onPointerMove = (e) => {
    if (!drag) return;
    const p = toSvg(e);
    if (drag.kind === 'pan') {
      setView((v) => ({ ...v, tx: drag.tx + (p.x - drag.x), ty: drag.ty + (p.y - drag.y) }));
    } else {
      const v = viewRef.current;
      const x = Math.round(Math.min(PLAN_W, Math.max(0, (p.x - v.tx) / v.scale - drag.dx)));
      const y = Math.round(Math.min(PLAN_H, Math.max(0, (p.y - v.ty) / v.scale - drag.dy)));
      setDragPos({ id: drag.id, x, y, moved: true });
    }
  };

  const endDrag = () => {
    if (drag?.kind === 'desk' && dragPos?.moved) onDeskMove?.(dragPos.id, dragPos.x, dragPos.y);
    setDrag(null);
    setDragPos(null);
  };

  const startPan = (e) => {
    const p = toSvg(e);
    svgRef.current.setPointerCapture?.(e.pointerId);
    setDrag({ kind: 'pan', x: p.x, y: p.y, tx: view.tx, ty: view.ty });
  };

  const startDeskDrag = (e, desk) => {
    if (mode !== 'edit') return;
    e.stopPropagation();
    const p = toSvg(e);
    const px = (p.x - view.tx) / view.scale;
    const py = (p.y - view.ty) / view.scale;
    svgRef.current.setPointerCapture?.(e.pointerId);
    setDrag({ kind: 'desk', id: desk.id, dx: px - desk.x, dy: py - desk.y });
    onDeskClick?.(desk.id);
  };

  const you = desks.filter((d) => states[d.id]?.mine);

  return (
    <div className="relative h-full w-full">
      <svg
        ref={svgRef}
        viewBox={`-10 -10 ${PLAN_W + 20} ${PLAN_H + 20}`}
        className={`h-full w-full touch-none select-none ${drag?.kind === 'pan' ? 'cursor-grabbing' : 'cursor-grab'}`}
        onPointerDown={startPan}
        onPointerMove={onPointerMove}
        onPointerUp={endDrag}
        onPointerCancel={endDrag}
        role="img"
        aria-label="Office floor plan"
      >
        <g transform={`translate(${view.tx} ${view.ty}) scale(${view.scale})`}>
          <Building showSafety={showSafety} />
          {desks.map((d) => {
            const pos = dragPos?.id === d.id ? dragPos : d;
            let fill;
            let interactive = true;
            let title = d.label;
            if (mode === 'edit') {
              fill = d.active ? C.available : C.unavailable;
              title = `${d.label}${d.active ? '' : ' (inactive)'} – drag to move`;
            } else {
              const st = states[d.id] || { state: 'unavailable' };
              fill = deskColor(st.mine ? 'booked' : st.state);
              interactive = st.state !== 'unavailable';
              title =
                st.state === 'booked'
                  ? `${d.label} – booked by ${st.bookedBy.join(', ')}`
                  : `${d.label} – ${st.state}`;
            }
            return (
              <DeskGlyph
                key={d.id}
                desk={d}
                x={pos.x}
                y={pos.y}
                fill={fill}
                title={title}
                interactive={interactive}
                outlined={d.id === selectedDeskId}
                onPointerDown={(e) => (mode === 'edit' ? startDeskDrag(e, d) : e.stopPropagation())}
                onClick={mode === 'view' && interactive ? () => onDeskClick?.(d.id) : undefined}
              />
            );
          })}
          {mode === 'edit' &&
            desks.map((d) => {
              const pos = dragPos?.id === d.id ? dragPos : d;
              return (
                <text key={`l-${d.id}`} x={pos.x} y={pos.y + 6} fontSize="11" fontWeight="600" textAnchor="middle" fill="#fff" pointerEvents="none">
                  {d.label}
                </text>
              );
            })}
          {highlight.map(({ deskId, label }) => {
            const d = desks.find((x) => x.id === deskId);
            if (!d) return null;
            return (
              <g key={`h-${deskId}`} pointerEvents="none">
                <circle className="pulse-ring" cx={d.x} cy={d.y} r="30" fill="none" stroke={C.selected} strokeWidth="3" />
                <Bubble x={d.x} y={d.y} text={label} color="#0F172A" />
              </g>
            );
          })}
          {mode === 'view' && you.map((d) => <Bubble key={`you-${d.id}`} x={d.x} y={d.y} text="YOU" />)}
        </g>
      </svg>

      <div className="absolute bottom-6 right-4 flex flex-col items-end gap-3">
      {extraControls}
      <div className="flex flex-col overflow-hidden rounded-md border border-slate-200 bg-white shadow-card">
        <button className="p-2 hover:bg-slate-50" onClick={() => zoomAt(1.25)} aria-label="Zoom in">
          <PlusIcon className="h-4 w-4" />
        </button>
        <button className="border-y border-slate-200 p-2 hover:bg-slate-50" onClick={() => setView({ scale: 1, tx: 0, ty: 0 })} aria-label="Reset view">
          <ResetIcon className="h-4 w-4" />
        </button>
        <button className="p-2 hover:bg-slate-50" onClick={() => zoomAt(1 / 1.25)} aria-label="Zoom out">
          <MinusIcon className="h-4 w-4" />
        </button>
      </div>
      </div>
    </div>
  );
}
