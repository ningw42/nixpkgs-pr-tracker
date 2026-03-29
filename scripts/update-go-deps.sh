# update-go-deps.sh — update Go module dependencies and refresh vendorHash in flake.nix
# Intended to run inside `nix develop` via the `update-go-deps` wrapper.

FLAKE_NIX="flake.nix"

if [[ ! -f "$FLAKE_NIX" ]]; then
  echo "error: $FLAKE_NIX not found — run this from the repo root" >&2
  exit 1
fi

echo "==> Updating Go dependencies..."
go get -u ./...
go mod tidy

echo "==> Invalidating vendorHash in $FLAKE_NIX..."
sed -i 's|vendorHash = "sha256-.*";|vendorHash = "";|' "$FLAKE_NIX"

echo "==> Building to compute new vendorHash (this will fail once)..."
BUILD_OUTPUT=$(nix build 2>&1) && {
  echo "==> Build succeeded — vendorHash was already correct (no dep changes?)"
  exit 0
}

NEW_HASH=$(echo "$BUILD_OUTPUT" | grep -oP 'got:\s+\Ksha256-[A-Za-z0-9+/]+=*' | head -1)

if [[ -z "$NEW_HASH" ]]; then
  echo "error: could not extract new hash from nix build output:" >&2
  echo "$BUILD_OUTPUT" >&2
  exit 1
fi

echo "==> New vendorHash: $NEW_HASH"
sed -i "s|vendorHash = \"\";|vendorHash = \"$NEW_HASH\";|" "$FLAKE_NIX"

echo "==> Verifying build with new hash..."
nix build

echo "==> Done. Changes:"
git diff --stat
