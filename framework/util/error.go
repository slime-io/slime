package util

type Error struct {
	M string
}

func (e Error) Error() string {
	return e.M
}
