package agent

// entityIDProviderAdapter adapts agent.EntityIDProvider to items.EntityIDProvider
type entityIDProviderAdapter struct {
	provider EntityIDProvider
}

func (a *entityIDProviderAdapter) GetEntityID() int32 {
	return a.provider.GetEntityID()
}
