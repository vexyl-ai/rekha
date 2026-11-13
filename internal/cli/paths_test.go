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
		if got := OutputPath(c.in, c.explicit, "svg"); got != c.want {
			t.Errorf("OutputPath(%q, %q) = %q, want %q", c.in, c.explicit, got, c.want)
		}
	}
	if got := OutputPath("dir/logo.png", "", "pdf"); got != "dir/logo.pdf" {
		t.Errorf("pdf: %q", got)
	}
}

func TestFormatOf(t *testing.T) {
	cases := []struct{ flag, output, want string }{
		{"", "", "svg"},
		{"", "out.pdf", "pdf"},
		{"", "out.EPS", "eps"},
		{"", "out.txt", "svg"},
		{"eps", "out.pdf", "eps"},
		{"", "-", "svg"},
	}
	for _, c := range cases {
		if got := FormatOf(c.flag, c.output); got != c.want {
			t.Errorf("FormatOf(%q, %q) = %q, want %q", c.flag, c.output, got, c.want)
		}
	}
}
