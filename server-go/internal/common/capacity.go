package common

// Capacity is a type-level representation of a legacy fixed capacity.
// Go does not permit integer constant type parameters, so each capacity is
// represented by a distinct zero-sized type.
type Capacity interface {
	WireCapacity() int
}

type (
	Cap32   struct{}
	Cap48   struct{}
	Cap64   struct{}
	Cap128  struct{}
	Cap256  struct{}
	Cap512  struct{}
	Cap1024 struct{}
)

func (Cap32) WireCapacity() int   { return 32 }
func (Cap48) WireCapacity() int   { return 48 }
func (Cap64) WireCapacity() int   { return 64 }
func (Cap128) WireCapacity() int  { return 128 }
func (Cap256) WireCapacity() int  { return 256 }
func (Cap512) WireCapacity() int  { return 512 }
func (Cap1024) WireCapacity() int { return 1024 }
