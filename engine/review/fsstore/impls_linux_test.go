package fsstore

func impls() []impl {
	return []impl{byPath(), {"held", Store.namesHeld, Store.createHeld}}
}
