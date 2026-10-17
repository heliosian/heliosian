package hosts

type App struct {
	Key   string
	Hosts []string
}

var Apps = []App{
	{Key: "home", Hosts: []string{"", "www", "home"}},
	{Key: "who", Hosts: []string{"who"}},
	{Key: "team", Hosts: []string{"team", "hca"}},
	{Key: "celebrate", Hosts: []string{"celebrate"}},
	{Key: "birthday", Hosts: []string{"birthday"}},
	{Key: "when", Hosts: []string{"when", "calendar", "cal"}},
	{Key: "loop", Hosts: []string{"loop"}},
	{Key: "ask", Hosts: []string{"ask"}},
	{Key: "admin", Hosts: []string{"admin"}},
	{Key: "wiki", Hosts: []string{"wiki"}},
	{Key: "mcp", Hosts: []string{"mcp"}},
}

func Qualify(label, domain string) string {
	if label == "" {
		return domain
	}
	return label + "." + domain
}

func AppOf(domain, host string) (string, bool) {
	for _, a := range Apps {
		for _, label := range a.Hosts {
			if Qualify(label, domain) == host {
				return a.Key, true
			}
		}
	}
	return "", false
}

func Origin(domain string) func(app string) string {
	return func(app string) string {
		for _, a := range Apps {
			if a.Key == app {
				return "https://" + Qualify(a.Hosts[0], domain)
			}
		}
		panic("no app " + app)
	}
}
