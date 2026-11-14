.PHONY: web test dev android clean

# Build the SvelteKit app and copy it where the Go core embeds it.
web:
	cd web && npm install --no-audit --no-fund && npm run build
	rm -rf core/webdist && mkdir -p core/webdist && cp -r web/build/. core/webdist/ && touch core/webdist/.gitkeep

test:
	cd core && go vet ./... && go test -race ./...
	cd web && npm run check && npm test

# Desktop dev: builds the web app and runs the core; open the URL it prints.
dev: web
	cd core && go run ./cmd/nascd

android: web
	./scripts/build-android.sh && ./scripts/install.sh

clean:
	rm -rf web/build web/.svelte-kit android/app/build android/app/libs/nasc.aar
