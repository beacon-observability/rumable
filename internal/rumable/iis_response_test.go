package rumable

import "testing"

func TestIISResponseIsExactlyOneSnapshot(t *testing.T) {
	valid := `{"rules":[],"preconditions":[],"compressionPresent":false}`
	for _, input := range []string{valid, "\xef\xbb\xbf" + valid + "\r\n"} {
		if _, err := decodeIISResponse([]byte(input)); err != nil {
			t.Fatal(err)
		}
	}
	for _, input := range []string{"Attributes : {name}\n" + valid, valid + "\nwarning", "[]", "null", "{}", valid + valid} {
		if _, err := decodeIISResponse([]byte(input)); err == nil {
			t.Fatalf("accepted invalid bridge output: %s", input)
		}
	}
}
