package config

// SignInConfig holds the credentials of the social sign-in providers (D-023). A provider is on only when both its
// client id and secret are set; setting just one is a configuration mistake and stops the start.
type SignInConfig struct {
	GoogleClientID     string
	GoogleClientSecret string
	GitHubClientID     string
	GitHubClientSecret string
}

func loadSignInConfig() SignInConfig {
	return SignInConfig{
		GoogleClientID: env("GOOGLE_CLIENT_ID", ""), GoogleClientSecret: env("GOOGLE_CLIENT_SECRET", ""),
		GitHubClientID: env("GITHUB_CLIENT_ID", ""), GitHubClientSecret: env("GITHUB_CLIENT_SECRET", ""),
	}
}

// GoogleSignIn reports whether Google sign-in is configured.
func (c SignInConfig) GoogleSignIn() bool {
	return c.GoogleClientID != "" && c.GoogleClientSecret != ""
}

// GitHubSignIn reports whether GitHub sign-in is configured.
func (c SignInConfig) GitHubSignIn() bool {
	return c.GitHubClientID != "" && c.GitHubClientSecret != ""
}

func (c *Config) validateSignIn() []string {
	var p []string
	if (c.GoogleClientID == "") != (c.GoogleClientSecret == "") {
		p = append(p, "GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET must be set together (both empty disables Google sign-in)")
	}
	if (c.GitHubClientID == "") != (c.GitHubClientSecret == "") {
		p = append(p, "GITHUB_CLIENT_ID and GITHUB_CLIENT_SECRET must be set together (both empty disables GitHub sign-in)")
	}
	return p
}
