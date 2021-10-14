package util

type Queue interface {
	Pop() interface{}
	Push(interface{})
	Peek() interface{}
	Length() int
}

type FILOStack struct {
	slice []interface{}
}

func (s *FILOStack) Length() int {
	return len(s.slice)
}

func NewFILOStack() *FILOStack {
	return &FILOStack{
		slice: make([]interface{}, 0),
	}
}

func (s *FILOStack) Pop() interface{} {
	if len(s.slice) == 0 {
		return nil
	}
	r := s.slice[len(s.slice)-1]
	s.slice = s.slice[:len(s.slice)-1]
	return r
}

func (s *FILOStack) Push(e interface{}) {
	s.slice = append(s.slice, e)
}

func (s *FILOStack) Peek() interface{} {
	if len(s.slice) == 0 {
		return nil
	}
	r := s.slice[len(s.slice)-1]
	return r
}

type FIFOStack struct {
	slice []interface{}
}

func (s *FIFOStack) Length() int {
	return len(s.slice)
}

func NewFIFOStack() *FIFOStack {
	return &FIFOStack{
		slice: make([]interface{}, 0),
	}
}

func (s *FIFOStack) Pop() interface{} {
	if len(s.slice) == 0 {
		return nil
	}
	r := s.slice[len(s.slice)-1]
	s.slice = s.slice[:len(s.slice)-1]
	return r
}

func (s *FIFOStack) Push(e interface{}) {
	s.slice = append(s.slice, e)
}

func (s *FIFOStack) Peek() interface{} {
	if len(s.slice) == 0 {
		return nil
	}
	r := s.slice[len(s.slice)-1]
	return r
}
