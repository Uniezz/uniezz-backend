package connections

type Mode string

const (
	ModeFriends    Mode = "friends"
	ModeStudyBuddy Mode = "studyBuddy"
	ModeDating     Mode = "dating"
)

func (mode Mode) valid() bool {
	return mode == ModeFriends || mode == ModeStudyBuddy || mode == ModeDating
}

type action string

const (
	actionLike action = "like"
	actionPass action = "pass"
)
