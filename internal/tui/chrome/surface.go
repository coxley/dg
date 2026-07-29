package chrome

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
)

// SurfaceID identifies one workspace surface.
type SurfaceID string

// SurfaceRole selects placement and input behavior.
type SurfaceRole uint8

const (
	// SurfaceDock reduces the workspace main rectangle.
	SurfaceDock SurfaceRole = iota
	// SurfaceFloating overlays the workspace.
	SurfaceFloating
	// SurfacePassive overlays without keyboard focus.
	SurfacePassive
	// SurfaceDrawer overlays constrained layouts.
	SurfaceDrawer
	// SurfaceModal blocks lower surfaces.
	SurfaceModal
)

// DockEdge selects the workspace edge consumed by a dock.
type DockEdge uint8

const (
	// DockLeft consumes columns from the left edge.
	DockLeft DockEdge = iota
	// DockRight consumes columns from the right edge.
	DockRight
	// DockTop consumes rows from the top edge.
	DockTop
	// DockBottom consumes rows from the bottom edge.
	DockBottom
)

// Anchor selects a surface coordinate space.
type Anchor uint8

const (
	// AnchorTerminal uses terminal coordinates.
	AnchorTerminal Anchor = iota
	// AnchorWorkspace uses workspace coordinates.
	AnchorWorkspace
	// AnchorCanvas uses canvas coordinates.
	AnchorCanvas
	// AnchorDock uses the declared dock rectangle.
	AnchorDock
)

// Surface declares application placement policy.
type Surface struct {
	ID             SurfaceID
	Role           SurfaceRole
	Anchor         Anchor
	Dock           DockEdge
	Requested      Rect
	Priority       int
	Visible        bool
	Animated       bool
	DismissOutside bool
	DismissBack    bool
	FocusOnOpen    bool
}

// SurfacePlan records requested and terminal-clamped geometry.
type SurfacePlan struct {
	Surface Surface
	Anchor  Rect
	Content Rect
	Rect    Rect
}

// WorkspacePlan is one arranged workspace and z-order.
type WorkspacePlan struct {
	Terminal Rect
	Main     Rect
	Canvas   Rect
	Dock     Rect
	Footer   Rect
	Surfaces []SurfacePlan
}

// ErrDuplicateSurface reports repeated semantic surface identities.
var ErrDuplicateSurface = errors.New("duplicate surface ID")

// DuplicateSurfaceError identifies one repeated surface identity.
type DuplicateSurfaceError struct {
	ID SurfaceID
}

func (e DuplicateSurfaceError) Error() string {
	return fmt.Sprintf("%v: %q", ErrDuplicateSurface, e.ID)
}

func (e DuplicateSurfaceError) Unwrap() error {
	return ErrDuplicateSurface
}

// Workspace retains surface placement and pointer capture.
type Workspace struct {
	terminal Rect
	footer   int
	surfaces []Surface
	capture  SurfaceID
	opened   SurfaceID
	plan     WorkspacePlan
	motions  map[SurfaceID]*cellTransition
	noMotion bool
}

// SetTerminal updates terminal geometry.
func (w *Workspace) SetTerminal(size Size) {
	w.terminal = Rect{Width: max(size.Width, 0), Height: max(size.Height, 0)}
	w.arrange()
}

// SetFooter reserves sticky workspace rows.
func (w *Workspace) SetFooter(height int) {
	w.footer = max(height, 0)
	w.arrange()
}

// SetSurfaces validates and replaces application surface declarations.
func (w *Workspace) SetSurfaces(surfaces []Surface) error {
	seen := make(map[SurfaceID]bool, len(surfaces))
	visible := make(map[SurfaceID]bool, len(w.surfaces))
	for _, surface := range w.surfaces {
		visible[surface.ID] = surface.Visible
	}
	w.opened = ""
	openedPriority := 0
	for _, surface := range surfaces {
		if surface.ID == "" || seen[surface.ID] {
			return DuplicateSurfaceError{ID: surface.ID}
		}
		seen[surface.ID] = true
		if surface.Visible && !visible[surface.ID] && surface.FocusOnOpen {
			if w.opened == "" || surface.Priority >= openedPriority {
				w.opened = surface.ID
				openedPriority = surface.Priority
			}
		}
	}
	w.surfaces = append(w.surfaces[:0], surfaces...)
	w.arrange()
	return nil
}

// RetargetSurface moves an animated surface toward extent cells.
func (w *Workspace) RetargetSurface(id SurfaceID, extent int) bool {
	if w.motions == nil {
		w.motions = make(map[SurfaceID]*cellTransition)
	}
	transition := w.motions[id]
	if transition == nil {
		transition = &cellTransition{}
		w.motions[id] = transition
	}
	moving := transition.retarget(extent, w.noMotion)
	w.arrange()
	return moving
}

// AdvanceSurface advances an animated surface to its next distinct cell.
func (w *Workspace) AdvanceSurface(id SurfaceID) bool {
	transition := w.motions[id]
	if transition == nil || !transition.advance() {
		return false
	}
	w.arrange()
	return true
}

// SurfacePosition returns an animated surface's current visible extent.
func (w *Workspace) SurfacePosition(id SurfaceID) int {
	if transition := w.motions[id]; transition != nil {
		return transition.position
	}
	return 0
}

// SurfaceMoving reports whether an animated surface has reached its target.
func (w *Workspace) SurfaceMoving(id SurfaceID) bool {
	if transition := w.motions[id]; transition != nil {
		return transition.position != transition.target
	}
	return false
}

// SetMotionEnabled controls whether animated surfaces transition or snap.
func (w *Workspace) SetMotionEnabled(enabled bool) {
	w.noMotion = !enabled
	if enabled {
		return
	}
	for _, transition := range w.motions {
		transition.position = transition.target
		transition.start = transition.target
		transition.frame = 0
		transition.frames = 0
	}
	w.arrange()
}

// PointerBlocked reports whether a modal prevents the canvas from receiving p.
func (w *Workspace) PointerBlocked(point Point) bool {
	for i := len(w.plan.Surfaces) - 1; i >= 0; i-- {
		surface := w.plan.Surfaces[i]
		if surface.Rect.Contains(point) {
			return false
		}
		if surface.Surface.Role == SurfaceModal {
			return true
		}
	}
	return false
}

// Plan returns the current workspace arrangement.
func (w *Workspace) Plan() WorkspacePlan {
	plan := w.plan
	plan.Surfaces = append([]SurfacePlan(nil), plan.Surfaces...)
	return plan
}

// Geometry returns workspace rectangles without copying surface plans.
func (w *Workspace) Geometry() WorkspacePlan {
	plan := w.plan
	plan.Surfaces = nil
	return plan
}

// Surface returns one arranged surface without copying the complete plan.
func (w *Workspace) Surface(id SurfaceID) (SurfacePlan, bool) {
	for _, surface := range w.plan.Surfaces {
		if surface.Surface.ID == id {
			return surface, true
		}
	}
	return SurfacePlan{}, false
}

// SurfaceAt returns the topmost pointer target, respecting modal blocking.
func (w *Workspace) SurfaceAt(point Point) (SurfaceID, bool) {
	if w.capture != "" {
		return w.capture, true
	}
	for i := len(w.plan.Surfaces) - 1; i >= 0; i-- {
		surface := w.plan.Surfaces[i]
		if surface.Rect.Contains(point) {
			return surface.Surface.ID, true
		}
		if surface.Surface.Role == SurfaceModal {
			return "", false
		}
	}
	return "", false
}

// DismissAt returns the top surface dismissed by an outside click.
func (w *Workspace) DismissAt(point Point) (SurfaceID, bool) {
	if w.capture != "" {
		return "", false
	}
	for i := len(w.plan.Surfaces) - 1; i >= 0; i-- {
		surface := w.plan.Surfaces[i]
		if surface.Rect.Contains(point) {
			return "", false
		}
		if surface.Surface.DismissOutside {
			return surface.Surface.ID, true
		}
		if surface.Surface.Role == SurfaceModal {
			return "", false
		}
	}
	return "", false
}

// Back returns the top surface whose declaration handles Back.
func (w *Workspace) Back() (SurfaceID, bool) {
	for i := len(w.plan.Surfaces) - 1; i >= 0; i-- {
		surface := w.plan.Surfaces[i].Surface
		if surface.DismissBack {
			return surface.ID, true
		}
		if surface.Role == SurfaceModal {
			return "", false
		}
	}
	return "", false
}

// FocusOnOpen returns the highest-priority surface newly opened by SetSurfaces.
func (w *Workspace) FocusOnOpen() (SurfaceID, bool) {
	return w.opened, w.opened != ""
}

// Capture retains pointer ownership until Release.
func (w *Workspace) Capture(id SurfaceID) {
	for _, surface := range w.plan.Surfaces {
		if surface.Surface.ID == id {
			w.capture = id
			return
		}
	}
}

// Release clears pointer capture.
func (w *Workspace) Release() {
	w.capture = ""
}

// Capture returns the current pointer owner.
func (w *Workspace) CaptureID() SurfaceID {
	return w.capture
}

// ScreenToCanvas translates a visible screen cell into canvas-host cells.
func (w *Workspace) ScreenToCanvas(point Point) (Point, bool) {
	if !w.plan.Canvas.Contains(point) {
		return Point{}, false
	}
	return Point{X: point.X - w.plan.Canvas.X, Y: point.Y - w.plan.Canvas.Y}, true
}

// CanvasToScreen translates a canvas-host cell into screen coordinates.
func (w *Workspace) CanvasToScreen(point Point) (Point, bool) {
	if point.X < 0 || point.Y < 0 ||
		point.X >= w.plan.Canvas.Width || point.Y >= w.plan.Canvas.Height {
		return Point{}, false
	}
	return Point{X: point.X + w.plan.Canvas.X, Y: point.Y + w.plan.Canvas.Y}, true
}

func (w *Workspace) arrange() {
	main := w.terminal
	footerHeight := min(w.footer, main.Height)
	main.Height -= footerHeight
	plan := WorkspacePlan{
		Terminal: w.terminal,
		Main:     main,
		Canvas:   main,
		Footer: Rect{
			Y:      main.Bottom(),
			Width:  w.terminal.Width,
			Height: footerHeight,
		},
	}
	surfaces := append([]Surface(nil), w.surfaces...)
	slices.SortStableFunc(surfaces, func(a, b Surface) int {
		return cmp.Compare(a.Priority, b.Priority)
	})
	var dockPlans []SurfacePlan
	for _, surface := range surfaces {
		if !surface.Visible || surface.Role != SurfaceDock {
			continue
		}
		anchor := plan.Canvas
		rect := dockRect(surface, anchor)
		if surface.Animated {
			rect = dockRectAtExtent(surface, anchor, w.SurfacePosition(surface.ID))
		}
		switch surface.Dock {
		case DockRight:
			plan.Canvas.Width -= rect.Width
		case DockTop:
			plan.Canvas.Y += rect.Height
			plan.Canvas.Height -= rect.Height
		case DockBottom:
			plan.Canvas.Height -= rect.Height
		case DockLeft:
			plan.Canvas.X += rect.Width
			plan.Canvas.Width -= rect.Width
		}
		plan.Dock = unionRect(plan.Dock, rect)
		dockPlans = append(dockPlans, SurfacePlan{
			Surface: surface,
			Anchor:  anchor,
			Content: rect,
			Rect:    rect,
		})
	}
	for _, surface := range surfaces {
		if !surface.Visible {
			continue
		}
		if surface.Role == SurfaceDock {
			for _, dock := range dockPlans {
				if dock.Surface.ID == surface.ID {
					plan.Surfaces = append(plan.Surfaces, dock)
					break
				}
			}
			continue
		}
		anchor := w.anchorRect(surface.Anchor, plan)
		if surface.Role == SurfaceDrawer && surface.Animated {
			content := drawerRect(surface, anchor, w.SurfacePosition(surface.ID))
			plan.Surfaces = append(plan.Surfaces, SurfacePlan{
				Surface: surface,
				Anchor:  anchor,
				Content: content,
				Rect:    surfaceIntersection(content, anchor),
			})
			continue
		}
		requested := surface.Requested
		requested.X += anchor.X
		requested.Y += anchor.Y
		rect := clampRect(requested, anchor)
		plan.Surfaces = append(plan.Surfaces, SurfacePlan{
			Surface: surface,
			Anchor:  anchor,
			Content: rect,
			Rect:    rect,
		})
	}
	if w.capture != "" {
		found := false
		for _, surface := range plan.Surfaces {
			found = found || surface.Surface.ID == w.capture
		}
		if !found {
			w.capture = ""
		}
	}
	w.plan = plan
}

func dockRectAtExtent(surface Surface, anchor Rect, extent int) Rect {
	requested := surface.Requested
	switch surface.Dock {
	case DockLeft, DockRight:
		requested.Width = min(max(extent, 0), requested.Width)
	case DockTop, DockBottom:
		requested.Height = min(max(extent, 0), requested.Height)
	}
	surface.Requested = requested
	return dockRect(surface, anchor)
}

func drawerRect(surface Surface, anchor Rect, extent int) Rect {
	width := min(max(surface.Requested.Width, 0), anchor.Width)
	height := min(max(surface.Requested.Height, 0), anchor.Height)
	switch surface.Dock {
	case DockRight:
		return Rect{
			X: anchor.Right() - min(max(extent, 0), width),
			Y: anchor.Y, Width: width, Height: height,
		}
	case DockTop:
		return Rect{
			X: anchor.X, Y: anchor.Y - height + min(max(extent, 0), height),
			Width: width, Height: height,
		}
	case DockBottom:
		return Rect{
			X: anchor.X, Y: anchor.Bottom() - min(max(extent, 0), height),
			Width: width, Height: height,
		}
	case DockLeft:
		return Rect{
			X: anchor.X - width + min(max(extent, 0), width),
			Y: anchor.Y, Width: width, Height: height,
		}
	default:
		return Rect{}
	}
}

func surfaceIntersection(a, b Rect) Rect {
	left := max(a.X, b.X)
	top := max(a.Y, b.Y)
	right := min(a.Right(), b.Right())
	bottom := min(a.Bottom(), b.Bottom())
	if right <= left || bottom <= top {
		return Rect{}
	}
	return Rect{X: left, Y: top, Width: right - left, Height: bottom - top}
}

func (w *Workspace) anchorRect(anchor Anchor, plan WorkspacePlan) Rect {
	switch anchor {
	case AnchorWorkspace:
		return plan.Main
	case AnchorCanvas:
		return plan.Canvas
	case AnchorDock:
		if plan.Dock.Width != 0 && plan.Dock.Height != 0 {
			return plan.Dock
		}
		return plan.Main
	case AnchorTerminal:
		return w.terminal
	default:
		return w.terminal
	}
}

func dockRect(surface Surface, anchor Rect) Rect {
	switch surface.Dock {
	case DockRight:
		width := min(max(surface.Requested.Width, 0), anchor.Width)
		return Rect{X: anchor.Right() - width, Y: anchor.Y, Width: width, Height: anchor.Height}
	case DockTop:
		height := min(max(surface.Requested.Height, 0), anchor.Height)
		return Rect{X: anchor.X, Y: anchor.Y, Width: anchor.Width, Height: height}
	case DockBottom:
		height := min(max(surface.Requested.Height, 0), anchor.Height)
		return Rect{X: anchor.X, Y: anchor.Bottom() - height, Width: anchor.Width, Height: height}
	case DockLeft:
		width := min(max(surface.Requested.Width, 0), anchor.Width)
		return Rect{X: anchor.X, Y: anchor.Y, Width: width, Height: anchor.Height}
	default:
		return Rect{}
	}
}

func unionRect(a, b Rect) Rect {
	if a.Width == 0 || a.Height == 0 {
		return b
	}
	if b.Width == 0 || b.Height == 0 {
		return a
	}
	left := min(a.X, b.X)
	top := min(a.Y, b.Y)
	right := max(a.Right(), b.Right())
	bottom := max(a.Bottom(), b.Bottom())
	return Rect{X: left, Y: top, Width: right - left, Height: bottom - top}
}

func clampRect(rect, parent Rect) Rect {
	rect.Width = min(max(rect.Width, 0), parent.Width)
	rect.Height = min(max(rect.Height, 0), parent.Height)
	rect.X = min(max(rect.X, parent.X), parent.Right()-rect.Width)
	rect.Y = min(max(rect.Y, parent.Y), parent.Bottom()-rect.Height)
	return rect
}
