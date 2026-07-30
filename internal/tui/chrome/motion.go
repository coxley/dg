package chrome

import (
	"math"
	"time"

	"github.com/charmbracelet/harmonica"
)

const (
	transitionFramesPerSecond = 60
	springFrequency           = 120
	springDamping             = 1.0
	settledPosition           = 0.001
	settledVelocity           = 0.01
	transitionStep            = time.Second / transitionFramesPerSecond
)

type scalarTransition struct {
	spring      harmonica.Spring
	position    float64
	velocity    float64
	target      float64
	accumulated time.Duration
	published   int
	active      bool
}

func newScalarTransition() *scalarTransition {
	return &scalarTransition{
		spring: harmonica.NewSpring(
			harmonica.FPS(transitionFramesPerSecond),
			springFrequency,
			springDamping,
		),
	}
}

func (t *scalarTransition) retarget(target int) bool {
	t.target = float64(max(target, 0))
	if t.position == t.target {
		t.snap()
		return false
	}
	if t.velocity*(t.target-t.position) <= 0 {
		t.velocity = 0
	}
	t.active = true
	return true
}

func (t *scalarTransition) advance(delta time.Duration) bool {
	if !t.active || delta <= 0 {
		return false
	}
	previous := t.published
	t.accumulated += delta
	for t.active && t.accumulated >= transitionStep {
		t.accumulated -= transitionStep
		position := t.position
		t.position, t.velocity = t.spring.Update(t.position, t.velocity, t.target)
		if crossedTarget(position, t.position, t.target) ||
			math.Abs(t.position-t.target) < settledPosition &&
				math.Abs(t.velocity) < settledVelocity {
			t.snap()
			break
		}
		t.published = max(int(math.Round(t.position)), 0)
	}
	return t.published != previous
}

func (t *scalarTransition) snap() {
	t.position = t.target
	t.velocity = 0
	t.accumulated = 0
	t.published = int(t.target)
	t.active = false
}

func (t *scalarTransition) extent() int {
	return t.published
}

func (t *scalarTransition) moving() bool {
	return t.active
}

func crossedTarget(previous, current, target float64) bool {
	return previous < target && current >= target ||
		previous > target && current <= target
}
