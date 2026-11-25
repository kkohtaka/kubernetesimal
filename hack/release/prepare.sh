#!/usr/bin/env bash
# Collects the unreleased change files into a version and updates the changelog.
#
# Usage: prepare.sh <changie binary> <version>
#
# The version is one such as v0.2.0, or "auto" to derive it from the kinds of the unreleased changes. The first
# release is v0.1.0 whatever those kinds are: Changie would count up from v0.0.0, and a first release that holds
# only fixes would come out as v0.0.1.
set -euo pipefail

changie=$1
version=$2
first_version=v0.1.0

if ! find .changes/unreleased -name '*.yaml' | grep -q .; then
  echo "There are no unreleased changes, so there is nothing to release." >&2
  exit 1
fi

if [ "${version}" = "auto" ] && [ "$("${changie}" latest)" = "v0.0.0" ]; then
  echo "Nothing has been released yet. The first release is ${first_version}."
  version=${first_version}
fi

"${changie}" batch "${version}"
"${changie}" merge
echo "Prepared $("${changie}" latest). Commit the result and open a pull request to release it."
