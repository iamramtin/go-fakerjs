package fakerjs

// ValidMethod reports whether method is a "module.method" reference Generate accepts, such as
// "person.firstName". It checks the format only; whether the module and method exist is decided by the
// engine's allowlist.
//
// The grammar is ^[a-zA-Z][a-zA-Z0-9]*\.[a-zA-Z][a-zA-Z0-9]*$ . It is implemented without regexp so that
// "$" cannot match before a trailing newline in any future edit.
func ValidMethod(method string) bool {
	i := 0
	for part := range 2 {
		if i >= len(method) || !isLetter(method[i]) {
			return false
		}

		i++
		for i < len(method) && (isLetter(method[i]) || (method[i] >= '0' && method[i] <= '9')) {
			i++
		}

		if part == 0 {
			if i >= len(method) || method[i] != '.' {
				return false
			}

			i++
		}
	}

	return i == len(method)
}

func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
