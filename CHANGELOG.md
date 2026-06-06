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
