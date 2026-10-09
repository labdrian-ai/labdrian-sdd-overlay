package main

import "testing"

func TestPipkgOptionsFromEnv(t *testing.T) {
	env := func(values map[string]string) func(string) string {
		return func(name string) string { return values[name] }
	}
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"unset", nil, ""},
		{"set", map[string]string{"LABDRIAN_PI_DEPLOY_REF": "feature"}, "feature"},
		{"padded", map[string]string{"LABDRIAN_PI_DEPLOY_REF": " origin/pr \n"}, "origin/pr"},
		{"blank", map[string]string{"LABDRIAN_PI_DEPLOY_REF": "  "}, ""},
		{"another variable", map[string]string{"DEPLOY_REF": "feature"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pipkgOptionsFromEnv(env(c.env)).DeployRef; got != c.want {
				t.Errorf("DeployRef = %q, want %q", got, c.want)
			}
		})
	}
}
