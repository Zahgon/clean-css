# Bug reports

Every piece of software has bugs, and squashing them should always be the priority.

If you spotted an incorrect behavior, knowing the following facts will help fixing it:

* What's your input CSS and expected output?
* Do you use clean-css as a library or through the `cleancss` command?
* What options do you pass to clean-css?
* What version of clean-css do you use?
* What operating system do you use?

# Pull requests

We love pull requests! To contribute to clean-css first fork, then clone the repo:

```shell
git clone git@github.com:your-username/clean-css.git
```

Make sure you have Go 1.23+ installed; clean-css uses the standard library only, so there is nothing else to download.

Make sure the tests pass:

```shell
go test ./...
```

Then add tests for your change. Check if tests fail. Make your change. Make the tests pass.

At the end make sure the code is formatted and passes the checks:

```shell
gofmt -l .
go vet ./...
```

Finally push to your fork and [submit a pull request](https://github.com/clean-css/clean-css/compare/).

At this point you're waiting for a PR review which should not thake more than a day.

Some things that will increase the chance that your pull request is accepted:

* Write tests.
* Write self-documenting code.
* Squash commits.
* Write a [good commit message](http://tbaggery.com/2008/04/19/a-note-about-git-commit-messages.html).
