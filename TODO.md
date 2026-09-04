# TODO

## v1.3.x

- [ ] Clean up the messy code
- [ ] Add search filters
- [x] Make a port for FreeBSD
- [ ] **Finish parsing the description**
- [x] Implement strips in daily arts
- [x] Fix the page navigation bug
- [x] Make proper error display
- [x] Make the units in the config more understandable
- [ ] Add an instance checker for availability
- [x] Add viewing of a user's liked arts
- [ ] Add the ability to embed templates into the binary `[P]`
- [x] Implement thumbnails and optimize CSS for small screens
- [ ] Write a Makefile and a script for automatic instance deployment
- [ ] Fix the emoji bug where some custom emotes may not display
- [x] Add a `&filename` argument that serves the file with a properly formatted name
- [x] Improve the caching system: add a rating for eviction and copy images into RAM

## v1.4

- [x] Implement an API
- [x] Implement themes
- [~] Switch to arenas in the cache — not doing this. An arena frees a whole
      region at once, and the media cache evicts entries individually by score
      (see `ageMemCache`), so entries would either outlive their eviction until
      the whole arena went, or need one arena each, which is allocation with
      extra bookkeeping. Arenas suit request-scoped allocation, not a
      long-lived scored cache. They also remain behind `GOEXPERIMENT=arenas`,
      unsupported and paused upstream, which every build and the Dockerfile
      would have to carry.
- [x] Implement a multilingual interface
