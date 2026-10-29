# Design reference analysis

Source: Storylane screenshot supplied by the product owner (floor-plan "MAP" view).

## Layout

| Region | Reference | Implementation |
|---|---|---|
| Left rail (~96px, white, faint right shadow) | Square blue logo tile; nav items stacked: **MAP** (active: pale-blue tile, blue icon + label, 3px blue bar on the rail's right edge), **BOOKINGS**, **USERS** (dark icon over uppercase bold label); round avatar "SV" with red/green presence dot at the bottom | `Sidebar.jsx`; ADMIN item added for administrators only |
| Top bar (no background, floats over canvas) | Date control: bordered 2px blue, rounded 6px — calendar icon, "Thu 13 Mar", blue circular ✕ (reset to today), ‹ › stepper. Round white filter button with shadow | `DateControl.jsx` (popover with rolling 60-day strip), `FilterButton` opens hour start/end selection |
| Top centre | White card with shadow: **Building** / *Floor* (grey) + green pill with available-desk count ("33") | `LocationBadge.jsx`, count from Python availability module |
| Top right | Wide rounded search field "Find a colleague" with magnifier | `ColleagueSearch.jsx` – highlights that colleague's desk |
| Canvas | White page; L-shaped building outline, fill very pale blue (#EAF4FC), walls ~4px mid blue (#2E7DC4), interior partitions thin blue lines | `FloorPlan.jsx` SVG, viewBox driven by settings |
| Desks | Clusters of 2×2 (sometimes 2×1) "desk + chair" glyphs: rounded rectangle desk with a semicircular chair bump. **Green (#66CC1F)** = available, **amber (#FDB813)** = booked | `DeskGlyph` with states available / selected / booked / unavailable / mine |
| "YOU" marker | Blue speech-bubble pill "YOU" anchored above the user's own booked desk | rendered for the signed-in user's booking on the selected date/time |
| Meeting rooms / amenities | Green "people around table" icons, emergency signs (first aid, extinguisher, exit) | decorative SVG layer (non-bookable), `Support` text label |
| Right edge | Collapsed side-panel tab (‹ people icon) | Booking confirmation / desk details drawer slides out here |
| Bottom right | Vertical control stack: first-aid toggle, emergency-exit toggle, zoom +, reset, zoom − | `MapControls.jsx` (zoom/pan on the SVG, amenity-layer toggles) |

## States (the reference shows only two; spec requires four)

| State | Colour | Notes |
|---|---|---|
| available | green #66CC1F | clickable |
| booked | amber #FDB813 | tooltip shows who (colleague search relies on this) |
| selected | blue #1E6FD9 with darker outline | the desk being booked in the drawer |
| unavailable | grey #C5CCD3 | disabled by admin, or outside the window |
| mine | amber + "YOU" bubble | matches reference |

## Typography

System sans-serif (Inter-like look, bundled locally – no Google Fonts), 14px controls, bold uppercase 12px rail labels, dark text #1F2937, muted #6B7280.
