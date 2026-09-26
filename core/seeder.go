package core

type Identifiable interface {
	ID() string
	Dependencies() []string
}
