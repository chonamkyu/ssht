package keys

import (
	"fmt"
	"strings"
)

var keyMap = map[string]byte{
	"ctrl+a":    0x01,
	"ctrl+b":    0x02,
	"ctrl+c":    0x03,
	"ctrl+d":    0x04,
	"ctrl+e":    0x05,
	"ctrl+f":    0x06,
	"ctrl+g":    0x07,
	"ctrl+h":    0x08,
	"tab":       0x09,
	"ctrl+i":    0x09,
	"enter":     0x0d,
	"ctrl+m":    0x0d,
	"ctrl+j":    0x0a,
	"ctrl+k":    0x0b,
	"ctrl+l":    0x0c,
	"ctrl+n":    0x0e,
	"ctrl+o":    0x0f,
	"ctrl+p":    0x10,
	"ctrl+q":    0x11,
	"ctrl+r":    0x12,
	"ctrl+s":    0x13,
	"ctrl+t":    0x14,
	"ctrl+u":    0x15,
	"ctrl+v":    0x16,
	"ctrl+w":    0x17,
	"ctrl+x":    0x18,
	"ctrl+y":    0x19,
	"ctrl+z":    0x1a,
	"escape":    0x1b,
	"esc":       0x1b,
	"backspace": 0x7f,
}

var specialKeys = map[string][]byte{
	"up":     {0x1b, '[', 'A'},
	"down":   {0x1b, '[', 'B'},
	"right":  {0x1b, '[', 'C'},
	"left":   {0x1b, '[', 'D'},
	"home":   {0x1b, '[', 'H'},
	"end":    {0x1b, '[', 'F'},
	"delete": {0x1b, '[', '3', '~'},
	"pgup":   {0x1b, '[', '5', '~'},
	"pgdown": {0x1b, '[', '6', '~'},
	"f1":     {0x1b, 'O', 'P'},
	"f2":     {0x1b, 'O', 'Q'},
	"f3":     {0x1b, 'O', 'R'},
	"f4":     {0x1b, 'O', 'S'},
	"f5":     {0x1b, '[', '1', '5', '~'},
	"f6":     {0x1b, '[', '1', '7', '~'},
	"f7":     {0x1b, '[', '1', '8', '~'},
	"f8":     {0x1b, '[', '1', '9', '~'},
	"f9":     {0x1b, '[', '2', '0', '~'},
	"f10":    {0x1b, '[', '2', '1', '~'},
	"f11":    {0x1b, '[', '2', '3', '~'},
	"f12":    {0x1b, '[', '2', '4', '~'},
}

func ParseKeys(input string) ([]byte, error) {
	var result []byte

	parts := strings.Split(input, ",")
	for _, part := range parts {
		key := strings.TrimSpace(strings.ToLower(part))

		if b, ok := keyMap[key]; ok {
			result = append(result, b)
			continue
		}

		if seq, ok := specialKeys[key]; ok {
			result = append(result, seq...)
			continue
		}

		if len(key) == 1 {
			result = append(result, key[0])
			continue
		}

		return nil, fmt.Errorf("unknown key: %s", part)
	}

	return result, nil
}

func ParseText(input string) []byte {
	replacer := strings.NewReplacer(
		`\n`, "\n",
		`\r`, "\r",
		`\t`, "\t",
		`\\`, "\\",
	)
	return []byte(replacer.Replace(input))
}
