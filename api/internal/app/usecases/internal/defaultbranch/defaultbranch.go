package defaultbranch

import "strings"

type RefRunner func(args ...string) (string, bool)

func Resolve(
	runner RefRunner,
	configList []string,
) string {
	if b := fromOriginHead(runner); b != "" {
		return b
	}
	if b := fromConfigList(runner, configList); b != "" {
		return b
	}
	if b := fromCurrentHead(runner); b != "" {
		return b
	}
	return neverEmptyFallback(configList)
}

func fromOriginHead(
	runner RefRunner,
) string {
	ref, ok := runner("symbolic-ref")
	if !ok || ref == "" {
		return ""
	}
	return lastPathSegment(ref)
}

func fromConfigList(
	runner RefRunner,
	configList []string,
) string {
	for _, branch := range configList {
		_, ok := runner("rev-parse", "--verify", branch)
		if ok {
			return branch
		}
	}
	return ""
}

func fromCurrentHead(
	runner RefRunner,
) string {
	ref, ok := runner("symbolic-ref", "HEAD")
	if !ok || ref == "" {
		return ""
	}
	return lastPathSegment(ref)
}

func lastPathSegment(
	ref string,
) string {
	parts := strings.Split(ref, "/")
	return parts[len(parts)-1]
}

func neverEmptyFallback(
	configList []string,
) string {
	if len(configList) > 0 {
		return configList[0]
	}
	return "main"
}
