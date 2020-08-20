package data

type Card struct {
	// A list of the values received, indexed by value.
	values []bool

	// How many values there will be in this Card, total.
	n      int
}

func NewCard(n int) *Card {
	return &Card{
		values: make([]bool, n),
		n:      0,
	}
}

func (c *Card) Track(v int) {
	if v >= len(c.values) {
		return
	}

	if !c.values[v] {
		c.values[v] = true
		c.n++
	}
}

func (c Card) Missing() int {
	return len(c.values) - c.n
}

func (c Card) MissingValues() []int {
	l := make([]int, 0, c.Missing())
	for v, ok := range c.values {
		if !ok {
			l = append(l, v)
		}
	}
	return l
}

func (c Card) Complete() bool {
	return c.n == len(c.values)
}

func (c Card) Ceiling() int {
	l := -1;
	for v, ok := range c.values {
		if !ok {
			break
		}
		l = v
	}
	return l
}
