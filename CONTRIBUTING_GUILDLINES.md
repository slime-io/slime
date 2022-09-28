# CONTRIBUTING GUILDLINES

Thank you for your interest in the slime program. Please take some time to learn about participation and communication in the slime program.

First, please review the [Code of Conduct](./CODE_OF_CONDUCT.md) file, and we will adhere to the principles therein.



In general, we encourage you to make communication as simple and easy as possible, so we will try to minimize the number of constraints, and the following is more of a process for informing and advising each other so that we can get the information we want easily.



## Communication



For more formal and definite communication content, such as bug reports, feature suggestions, etc., we recommend using [issue](https://github.com/slime-io/slime/issues/new/choose). For others, such as usage issues, feature questions, etc., a more flexible communication method is recommended, see [Community](./README.md#Community).



For the issue method, we have preset several types and created different content format templates, please fill them according to the format, so that others can understand the main points of the content faster.



## Participation



If you are interested in participating in the slime project, you can find appropriate content in [issues](https://github.com/slime-io/slime/issues), and participation in the issue discussion is also a good form.

If you wish to participate in the development, you can look for a feature with [feature](https://github.com/slime-io/slime/issues?q=is%3Aopen+is%3Aissue+label%3Afeature) or [bug](https://github.com/slime-io/slime/issues?q=is%3Aopen+is%3Aissue+label%3Afeature+label%3Abug) label and judge whether it is suitable for you according to the description therein. If you decide to take over, you can reply below the issue, and our maintainers will determine it as soon as possible to avoid duplication of work.

Of course, if there is already clear content, you can directly follow the process below to submit a PR.



### PR submission process

1. first, do [fork](https://github.com/slime-io/slime/fork) on the slime project

2. do some initialization work in the local repository

   1. add slime remote

      ```sh
      git remote add upstream git@github.com:slime-io/slime.git
      ```

   2. rebase slime remote

      ```sh
      git checkout master
      git pull --rebase upstream master
      ```

   3. create a new branch, suggest an ideographic name, such as `feature-xxx`, `fix-xxx`

      ```sh
      git checkout -b feature-xx
      ```

3. Complete the corresponding code changes, commit, and push to your remote repository

   ```sh
   # apply changes
   # add 
   # commit
   git push -f origin feature-xx
   ```

   

4. Click `New pull request` on the `Pull requests` page of your own remote repository (e.g. `github.com/xxx/slime/pull`). Select the head repository branch as the development branch such as `feature-xx`, then click `Create pull request`, fill in the appropriate title and description content and click `Create pull request` again.

   You can refer to the sample PR: https://github.com/slime-io/slime/pull/179



### Communication after submitting a PR

We will arrange a reviewer as soon as possible after the contributor submits the PR. During the review process, there may be some feedback comments or requests for changes, so please check the system notifications and emails. The reviewer will approve the PR after making the corresponding changes until the differences disappear and agreement is reached, and then the maintainer with permission will carry out the merge operation.

The process is as follows.

1. contributor -> PR
2. feedback/request change <- reviewer
3. contributor -> change(recommit, repush)
4. feedback/request change <- reviewer
5. ...
6. approve <- reviewer
7. merge <- maintainer

