package app

// CrowbarHome is the Crowbar home the adapter layer was opened against.
// Handlers that derive entity paths must use it, not resolve a home of their
// own, so one overridden home never splits the tree.
func (c *Container) CrowbarHome() string {
	return c.crowbarHome
}
