# LitmusGo:

- This repo consists of Litmus Chaos Experiments written in golang. The examples in this repo are good indicators
of how to construct the experiments in golang: complete with steady state checks, chaosresult generation, chaos injection etc..,
post chaos checks, create events and reports for observability and configure sinks for these.

## Run E2E on a Pull Request

- We can run a certain number of custom tests on a PR using GitHub chaos actions read about [custom bot](https://github.com/litmuschaos/litmus-go/blob/master/.github/workflows/guide.md) to know more.

**NOTE**

- This repo can be viewed as an extension to the [litmuschaos/litmus](https://github.com/litmuschaos/litmus) repo
  in the sense that the litmus repo also houses a significant set of experiments, built using ansible. The litmus repo
  will also continue to be the project's community-facing meta repo housing other important project artifacts. In that
  sense, litmus-go is very similar to and therefore a sister repo of [litmus-python](https://github.com/litmuschaos/litmus-python) which
  houses examples for experiment business logic written in python.


