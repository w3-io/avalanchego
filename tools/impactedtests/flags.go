package main

type multiFlag []string

func (m *multiFlag) String() string {
	if m == nil {
		return ""
	}
	return ""
}

func (m *multiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}
