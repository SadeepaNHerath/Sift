## Description

<!-- Briefly describe the rationale, architectural impact, and design decisions of your changes. -->

## Type of Change

- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Performance optimization or memory reduction
- [ ] Documentation update
- [ ] Formula or CI/CD workflow enhancement

## Safety & Quality Checklist

- [ ] My code adheres to the project's formatting guidelines (`gofmt -d .` returns 0 diff).
- [ ] I have executed `go vet ./...` and `go test -race ./...` and all tests pass locally.
- [ ] I have maintained documentation integrity (preserved docstrings, added comments where needed).
- [ ] My changes respect the non-root safety invariants and SIP/system path denylists.
- [ ] Any new interactive CLI features degrade gracefully in non-interactive/headless environments.
