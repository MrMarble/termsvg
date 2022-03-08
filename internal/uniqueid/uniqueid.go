package uniqueid

type ID []rune

func New() *ID {
	var id ID = []rune{'a'}

	return &id
}

func (id *ID) String() string {
	return string(*id)
}

func (id *ID) Next() {
	for i := len(*id) - 1; i >= 0; i-- {
		cur := (*id)[i]
		if cur < 'z' {
			(*id)[i]++
			return
		}

		if i == len(*id)-1 {
			(*id)[i] = 'a'
		}

		if i == 0 {
			*id = append(*id, 'a')
		}
	}
}
