## [2.1.1](https://github.com/cedricfarinazzo/k8s-nyx/compare/v2.1.0...v2.1.1) (2026-06-08)


### Bug Fixes

* **controller:** stop logging transient reconcile errors as failures ([#67](https://github.com/cedricfarinazzo/k8s-nyx/issues/67)) ([4c696c5](https://github.com/cedricfarinazzo/k8s-nyx/commit/4c696c55cec4ed8604fb73a7ff183fb92d310bc4))

# [2.1.0](https://github.com/cedricfarinazzo/k8s-nyx/compare/v2.0.2...v2.1.0) (2026-06-08)


### Features

* **chart:** expose metrics Service and verify it in helm test ([#66](https://github.com/cedricfarinazzo/k8s-nyx/issues/66)) ([8041a37](https://github.com/cedricfarinazzo/k8s-nyx/commit/8041a37ef15ebb37d00451f01f63771ed94d2139))

## [2.0.2](https://github.com/cedricfarinazzo/k8s-nyx/compare/v2.0.1...v2.0.2) (2026-06-07)


### Bug Fixes

* **deps:** upgrade to k8s 0.36 / controller-runtime 0.24 (go 1.26) ([#64](https://github.com/cedricfarinazzo/k8s-nyx/issues/64)) ([7fb7751](https://github.com/cedricfarinazzo/k8s-nyx/commit/7fb7751d7111465f44c0ece2f6777aba1e74e15e))

## [2.0.1](https://github.com/cedricfarinazzo/k8s-nyx/compare/v2.0.0...v2.0.1) (2026-06-07)


### Bug Fixes

* **deps:** bump k8s 0.35 / controller-runtime 0.23 (go 1.25 tier) ([#61](https://github.com/cedricfarinazzo/k8s-nyx/issues/61)) ([7376466](https://github.com/cedricfarinazzo/k8s-nyx/commit/7376466199ade2f205e8cb7ed6b0e3c659089ab9)), closes [#59](https://github.com/cedricfarinazzo/k8s-nyx/issues/59)

# [2.0.0](https://github.com/cedricfarinazzo/k8s-nyx/compare/v1.8.0...v2.0.0) (2026-06-07)


* feat(wake)!: simplify the wake override to a single expiry value ([#47](https://github.com/cedricfarinazzo/k8s-nyx/issues/47)) ([527b77b](https://github.com/cedricfarinazzo/k8s-nyx/commit/527b77b15e4b9fe80d9db414548540cd93bf5b68))


### BREAKING CHANGES

* wake overrides are now a single 'wake' key holding only the expiry; the previous multi-key '<expiry>;by=;reason=' format and per-entry attribution are removed.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>

# [1.8.0](https://github.com/cedricfarinazzo/k8s-nyx/compare/v1.7.0...v1.8.0) (2026-06-07)


### Features

* **rbac:** tighten operator RBAC to least privilege ([#41](https://github.com/cedricfarinazzo/k8s-nyx/issues/41)) ([836a0c7](https://github.com/cedricfarinazzo/k8s-nyx/commit/836a0c743df8f65fbe3ba39927005110a710ffaa))

# [1.7.0](https://github.com/cedricfarinazzo/k8s-nyx/compare/v1.6.0...v1.7.0) (2026-06-07)


### Features

* **ha:** harden leader election for high availability ([#40](https://github.com/cedricfarinazzo/k8s-nyx/issues/40)) ([7e0ffc6](https://github.com/cedricfarinazzo/k8s-nyx/commit/7e0ffc69d5b4d2efb776fff21af72b47e5c9374c))

# [1.6.0](https://github.com/cedricfarinazzo/k8s-nyx/compare/v1.5.0...v1.6.0) (2026-06-07)


### Features

* **metrics:** expose Prometheus metrics for sleep/wake behaviour ([#37](https://github.com/cedricfarinazzo/k8s-nyx/issues/37)) ([1f65c1d](https://github.com/cedricfarinazzo/k8s-nyx/commit/1f65c1d82e831c90d7798e7f45656905965ac07e))

# [1.5.0](https://github.com/cedricfarinazzo/k8s-nyx/compare/v1.4.0...v1.5.0) (2026-06-06)


### Features

* **audit:** structured JSON logs and SleepSchedule Events for lifecycle actions ([#36](https://github.com/cedricfarinazzo/k8s-nyx/issues/36)) ([8044674](https://github.com/cedricfarinazzo/k8s-nyx/commit/8044674e13f2082c13c2817a0ac4cfdc24586e3f))

# [1.4.0](https://github.com/cedricfarinazzo/k8s-nyx/compare/v1.3.0...v1.4.0) (2026-06-06)


### Features

* **workload:** refuse to sleep StatefulSet with whenScaled=Delete unless opted in ([#35](https://github.com/cedricfarinazzo/k8s-nyx/issues/35)) ([b5e785e](https://github.com/cedricfarinazzo/k8s-nyx/commit/b5e785e2db8d8dfc1d3ddc0f93507dec6184588a))

# [1.3.0](https://github.com/cedricfarinazzo/k8s-nyx/compare/v1.2.0...v1.3.0) (2026-06-06)


### Features

* **workload:** neutralize & restore HPA min/max on sleep ([#32](https://github.com/cedricfarinazzo/k8s-nyx/issues/32)) ([f0dac8d](https://github.com/cedricfarinazzo/k8s-nyx/commit/f0dac8d31eeeb2fa9d619567629edd1ad464e0d9))

# [1.2.0](https://github.com/cedricfarinazzo/k8s-nyx/compare/v1.1.0...v1.2.0) (2026-06-06)


### Features

* **workload:** sleep & restore CronJob and Job via spec.suspend ([#31](https://github.com/cedricfarinazzo/k8s-nyx/issues/31)) ([21a4342](https://github.com/cedricfarinazzo/k8s-nyx/commit/21a4342d0f5577f0107317e4d551c2ca0bb0dd62))

# [1.1.0](https://github.com/cedricfarinazzo/k8s-nyx/compare/v1.0.1...v1.1.0) (2026-06-06)


### Features

* **workload:** gate workload kinds via a handler registry ([#27](https://github.com/cedricfarinazzo/k8s-nyx/issues/27)) ([c727235](https://github.com/cedricfarinazzo/k8s-nyx/commit/c727235adb6dc6bb5fb4751be3d8583f51146c9f))
* **workload:** sleep & restore DaemonSet via sentinel nodeSelector ([#30](https://github.com/cedricfarinazzo/k8s-nyx/issues/30)) ([aeb0e8f](https://github.com/cedricfarinazzo/k8s-nyx/commit/aeb0e8fc276d5aae83446c86bac0bdc5d21c6280))

## [1.0.1](https://github.com/cedricfarinazzo/k8s-nyx/compare/v1.0.0...v1.0.1) (2026-06-06)


### Bug Fixes

* **target:** scope excludeRefs to a namespace ([#20](https://github.com/cedricfarinazzo/k8s-nyx/issues/20)) ([ef92beb](https://github.com/cedricfarinazzo/k8s-nyx/commit/ef92beb0383f65eabb2a6b95cf9cd367616fab7b))

# 1.0.0 (2026-06-06)


### Features

* **api:** define SleepSchedule CRD types and validation ([#4](https://github.com/cedricfarinazzo/k8s-nyx/issues/4)) ([238cd2b](https://github.com/cedricfarinazzo/k8s-nyx/commit/238cd2bb07a19b559f86e8e57715c696ffcad6cf))
* **operator:** auto-create and watch the per-schedule Wake ConfigMap ([#9](https://github.com/cedricfarinazzo/k8s-nyx/issues/9)) ([72de201](https://github.com/cedricfarinazzo/k8s-nyx/commit/72de2014de2053e8f7b76a3222850df9c05891cc))
* **operator:** emit Events and make reconcile idempotent ([#8](https://github.com/cedricfarinazzo/k8s-nyx/issues/8)) ([93878c8](https://github.com/cedricfarinazzo/k8s-nyx/commit/93878c8ef09f3c56fc61618d4c7810ee64d9a4a0))
* **operator:** force-awake on active wakes and self-clean expired entries ([#12](https://github.com/cedricfarinazzo/k8s-nyx/issues/12)) ([039f203](https://github.com/cedricfarinazzo/k8s-nyx/commit/039f203ff95dc879af06de5499ca04fd65e615ff))
* **operator:** sleep/wake workloads with exact-restore Checkpoint Secret ([#7](https://github.com/cedricfarinazzo/k8s-nyx/issues/7)) ([8a860e5](https://github.com/cedricfarinazzo/k8s-nyx/commit/8a860e5095ed32f14fbcebe9c14e74348e159fea))
* **schedule:** evaluate awake/asleep phase and next transition ([#5](https://github.com/cedricfarinazzo/k8s-nyx/issues/5)) ([4d184d0](https://github.com/cedricfarinazzo/k8s-nyx/commit/4d184d08e1a675328831d1dcfc6d922894318726))
* **target:** resolve workloads by namespaces/labels with exclusions ([#6](https://github.com/cedricfarinazzo/k8s-nyx/issues/6)) ([d252c56](https://github.com/cedricfarinazzo/k8s-nyx/commit/d252c560645c87b7415e8c759248f7b3958fa9ec))
* **wake:** parse wake entries and surface malformed input ([#10](https://github.com/cedricfarinazzo/k8s-nyx/issues/10)) ([39c0b2d](https://github.com/cedricfarinazzo/k8s-nyx/commit/39c0b2df5b6afab7cd15a588855839819f87d8b7))
* **wake:** resolve +duration, apply default, clamp to maxDuration ([#11](https://github.com/cedricfarinazzo/k8s-nyx/issues/11)) ([c65610e](https://github.com/cedricfarinazzo/k8s-nyx/commit/c65610e8f0aff4ab1f86f183ce0753239bba1158))
