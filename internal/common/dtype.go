package common

func IsValidDtypeSize(sz int) bool {
	switch sz {
	case 1, 2, 4:
		return true
	default:
		return false
	}
}
