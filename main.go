// Heliosian serves the Helios school community apps in production. Local
// development against sample data runs through cmd/startserver instead.
package main

import "heliosian/internal/app"

func main() {
	app.Serve(app.Production())
}
