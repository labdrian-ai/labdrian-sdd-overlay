# Issue Creation Procedure

## Variables

Split `$TARGET` into `HOST` (default `github.com`) and `REPO` (`OWNER/REPO`).

## Steps

1. Search open and closed issues: `gh issue list --repo "$TARGET" --state all --search "$QUERY" --limit 1000`. A saturated result is incomplete.
2. Of the results, take only the candidates whose title matches the request, at most 10; more than 10 is a stop that reports them and asks the owner. Read each candidate from the target host with `gh issue view "$N" --repo "$TARGET" --json number,url,title,body` and compare it with the selected form.
3. Fill the body file from the form's controls and the reviewed answers.
4. Run the privacy scan, then make one attempt: `gh issue create --repo "$TARGET" --title "$TITLE" --body-file "$BODY_FILE"`, or `gh issue comment "$NUMBER" --repo "$TARGET" --body-file "$BODY_FILE"`.
5. Take `NUMBER` from the issue URL that `gh issue create` prints. Take `COMMENT_ID` from the comment URL that `gh issue comment` prints, the number after `#issuecomment-`. A missing or unparsable URL is `unknown`: no read-back is possible.
6. Read an issue back with `gh issue view "$NUMBER" --repo "$TARGET" --json number,url,title,body`. Read a comment back with `gh api --hostname "$HOST" "repos/$REPO/issues/comments/$COMMENT_ID"`.

## Read-back comparison

The issue's title and body must equal the submitted title and body file. The comment's body must equal the submitted comment body file. A mismatch, or a read-back that fails, is `unknown`. Compare after turning CRLF into LF and dropping trailing white space at the end of each line and of the text, since GitHub may normalize both.
