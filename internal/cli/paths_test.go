package cli

import "testing"

func TestOutputPath(t *testing.T) {
	cases := []struct{ in, explicit, want string }{
		{"-", "", "-"},
		{"logo.png", "", "logo.svg"},
		{"dir/photo.jpeg", "", "dir/photo.svg"},
		{"noext", "", "noext.svg"},
		{"a.png", "out/b.svg", "out/b.svg"},
		{"-", "x.svg", "x.svg"},
	}
	for _, c := range cases {
		if got := OutputPath(c.in, c.explicit); got != c.want {
			t.Errorf("OutputPath(%q, %q) = %q, want %q", c.in, c.explicit, got, c.want)
		}
	}
}
