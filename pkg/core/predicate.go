package core

// Predicate filters events before they trigger reconciliation.
// This is inspired by controller-runtime's Predicate interface.
type Predicate interface {
	// Create returns true if the create event should trigger reconciliation.
	Create(key ResourceKey) bool

	// Update returns true if the update event should trigger reconciliation.
	Update(key ResourceKey, oldGeneration, newGeneration int64) bool

	// Delete returns true if the delete event should trigger reconciliation.
	Delete(key ResourceKey) bool
}

// PredicateFunc is a function type that implements all Predicate methods.
type PredicateFunc struct {
	CreateFunc func(key ResourceKey) bool
	UpdateFunc func(key ResourceKey, oldGeneration, newGeneration int64) bool
	DeleteFunc func(key ResourceKey) bool
}

// Create implements Predicate.
func (p PredicateFunc) Create(key ResourceKey) bool {
	if p.CreateFunc != nil {
		return p.CreateFunc(key)
	}
	return true
}

// Update implements Predicate.
func (p PredicateFunc) Update(key ResourceKey, oldGeneration, newGeneration int64) bool {
	if p.UpdateFunc != nil {
		return p.UpdateFunc(key, oldGeneration, newGeneration)
	}
	return true
}

// Delete implements Predicate.
func (p PredicateFunc) Delete(key ResourceKey) bool {
	if p.DeleteFunc != nil {
		return p.DeleteFunc(key)
	}
	return true
}

// GenerationChangedPredicate only allows events where the generation changed.
// This filters out status-only updates.
type GenerationChangedPredicate struct{}

// Create always returns true for new resources.
func (p GenerationChangedPredicate) Create(_ ResourceKey) bool {
	return true
}

// Update returns true only if the generation changed.
func (p GenerationChangedPredicate) Update(_ ResourceKey, oldGeneration, newGeneration int64) bool {
	return oldGeneration != newGeneration
}

// Delete always returns true for deleted resources.
func (p GenerationChangedPredicate) Delete(_ ResourceKey) bool {
	return true
}

// And combines multiple predicates with AND logic.
func And(predicates ...Predicate) Predicate {
	return PredicateFunc{
		CreateFunc: func(key ResourceKey) bool {
			for _, p := range predicates {
				if !p.Create(key) {
					return false
				}
			}
			return true
		},
		UpdateFunc: func(key ResourceKey, oldGen, newGen int64) bool {
			for _, p := range predicates {
				if !p.Update(key, oldGen, newGen) {
					return false
				}
			}
			return true
		},
		DeleteFunc: func(key ResourceKey) bool {
			for _, p := range predicates {
				if !p.Delete(key) {
					return false
				}
			}
			return true
		},
	}
}

// Or combines multiple predicates with OR logic.
func Or(predicates ...Predicate) Predicate {
	return PredicateFunc{
		CreateFunc: func(key ResourceKey) bool {
			for _, p := range predicates {
				if p.Create(key) {
					return true
				}
			}
			return false
		},
		UpdateFunc: func(key ResourceKey, oldGen, newGen int64) bool {
			for _, p := range predicates {
				if p.Update(key, oldGen, newGen) {
					return true
				}
			}
			return false
		},
		DeleteFunc: func(key ResourceKey) bool {
			for _, p := range predicates {
				if p.Delete(key) {
					return true
				}
			}
			return false
		},
	}
}
