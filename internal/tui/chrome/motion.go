package chrome

type cellTransition struct {
	position int
	start    int
	target   int
	frame    int
	frames   int
}

func (t *cellTransition) retarget(target int, disabled bool) bool {
	target = max(target, 0)
	if disabled {
		t.position = target
		t.start = target
		t.target = target
		t.frame = 0
		t.frames = 0
		return false
	}
	if target == t.target {
		return t.position != t.target
	}
	t.start = t.position
	t.target = target
	t.frame = 0
	t.frames = max(abs(target-t.position), 1)
	return t.position != t.target
}

func (t *cellTransition) advance() bool {
	if t.position == t.target {
		return false
	}
	previous := t.position
	for t.frame < t.frames && t.position == previous {
		t.frame++
		distance := t.target - t.start
		numerator := t.frame * t.frame
		if distance > 0 {
			numerator = 2*t.frame*t.frames - numerator
		}
		denominator := t.frames * t.frames
		t.position = t.start + roundedRatio(distance*numerator, denominator)
	}
	if t.frame == t.frames {
		t.position = t.target
	}
	return t.position != previous
}

func roundedRatio(numerator, denominator int) int {
	if numerator < 0 {
		return -roundedRatio(-numerator, denominator)
	}
	return (numerator + denominator/2) / denominator
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
