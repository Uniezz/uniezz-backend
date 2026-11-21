package chat

func (a accessRow) viewable() error {
	if a.Kind == KindDirect && !a.DirectVisible {
		return ErrNotFound
	}
	return nil
}

func (a accessRow) ownerOnly() error {
	if err := a.viewable(); err != nil {
		return err
	}
	if a.Role != RoleOwner {
		return ErrForbidden
	}
	return nil
}

func (a accessRow) groupOnly() error {
	if err := a.viewable(); err != nil {
		return err
	}
	if a.Kind != KindGroup {
		return ErrInvalidInput
	}
	return nil
}
