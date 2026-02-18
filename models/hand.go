package models

type Hand int32

const (
	MainHand Hand = 0
	OffHand  Hand = 1
)

func (h Hand) String() string {
	switch h {
	case MainHand:
		return "MainHand"
	case OffHand:
		return "OffHand"
	default:
		return "UnknownHand"
	}
}
