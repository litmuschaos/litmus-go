# LitmusGo:

[![Slack Channel](https://img.shields.io/badge/Slack-Join-purple)](https://slack.litmuschaos.io)
![GitHub Workflow](https://github.com/litmuschaos/litmus-go/actions/workflows/push.yml/badge.svg?branch=master)
[![Docker Pulls](https://img.shields.io/docker/pulls/litmuschaos/go-runner.svg)](https://hub.docker.com/r/litmuschaos/go-runner)
[![GitHub issues](https://img.shields.io/github/issues/litmuschaos/litmus-go)](https://github.com/litmuschaos/litmus-go/issues)
[![Twitter Follow](https://img.shields.io/twitter/follow/litmuschaos?style=social)](https://twitter.com/LitmusChaos)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/5297/badge)](https://bestpractices.coreinfrastructure.org/projects/5297)
[![Go Report Card](https://goreportcard.com/badge/github.com/litmuschaos/litmus-go)](https://goreportcard.com/report/github.com/litmuschaos/litmus-go)
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Flitmuschaos%2Flitmus-go.svg?type=shield)](https://app.fossa.io/projects/git%2Bgithub.com%2Flitmuschaos%2Flitmus-go?ref=badge_shield)
[![YouTube Channel](https://img.shields.io/badge/YouTube-Subscribe-red)](https://www.youtube.com/channel/UCa57PMqmz_j0wnteRa9nCaw)
<br><br>

This repo consists of Litmus Chaos Experiments written in golang. The examples in this repo are good indicators of how to construct the experiments in golang: complete with steady state checks, chaosresult generation, chaos injection etc.., post chaos checks, create events and reports for observability and configure sinks for these.

**NOTE**: This repo can be viewed as an extension to the [litmuschaos/litmus](https://github.com/litmuschaos/litmus) repo. The litmus repo will also continue to be the project's community-facing meta repo housing other important project artifacts. In that sense, litmus-go is very similar to and therefore a sister repo of [litmus-python](https://github.com/litmuschaos/litmus-python) which houses examples for experiment business logic written in python.

## Litmus SDK

The Litmus SDK provides a simple way to bootstrap your experiment and helps create the aforementioned artifacts in the appropriate directory (i.e., as per the chaos-category) based on an attributes file provided as input by the chart-developer. The scaffolded files consist of placeholders which can then be filled as desired.

It generates the custom chaos experiments with some default Pre & Post Chaos Checks (AUT & Auxiliary Applications status checks). It can use the existing chaoslib (present inside /chaoslib directory), if available else It will create a new chaoslib inside the corresponding directory.

Refer [Litmus-SDK](https://github.com/litmuschaos/litmus-go/blob/master/contribute/developer-guide/README.md) for more details.

## How to get started?

Refer the [LitmusChaos Docs](https://docs.litmuschaos.io) and [Experiment Docs](https://litmuschaos.github.io/litmus/experiments/categories/contents/)

## How do I contribute?

You can contribute by raising issues, improving the documentation, contributing to the core framework and tooling, etc.

Head over to the [Contribution guide](CONTRIBUTING.md)

## License
[![FOSSA Status](https://app.fossa.io/api/projects/git%2Bgithub.com%2Flitmuschaos%2Flitmus-go.svg?type=large)](https://app.fossa.io/projects/git%2Bgithub.com%2Flitmuschaos%2Flitmus-go?ref=badge_large)
