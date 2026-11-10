# Journal non-regular path rejection

The journal and sidecar lock preflight now rejects any existing filesystem object that is not a regular file, including directories and symbolic links reported by `Lstat`. Portable disposable-directory regression fixtures exercise both paths without opening a blocking FIFO or device.

**Boundary:** This remains a preflight only. It cannot prevent replacement after `Lstat`, parent-directory redirection, Windows reparse-point races, hard-link aliasing, hostile lock deletion, journal rollback, or power-loss corruption. `O_EXCL` lock creation and the current append path remain cooperative and are not a secure mount authorization mechanism. Source plus committed tests are not live-host qualification.
