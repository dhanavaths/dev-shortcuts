#!/bin/bash
set -e

TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROMPT_SCRIPT="${TEST_DIR}/../PS.sh"

fail() {
    printf 'FAIL: %s\n' "$*" >&2
    exit 1
}

(
    source "$PROMPT_SCRIPT"
    mount() {
        printf '%s\n' 'C: on /c type ntfs'
        if [[ "$drive_layout" == extra ]]; then
            printf '%s\n' 'Q: on /q type refs'
        fi
    }

    for OSTYPE in msys cygwin; do
        for MSYSTEM in MINGW64 MINGW32 UCRT64 CLANG64 CLANGARM64 MSYS; do
            for drive_layout in c-only extra; do
                expected=SAW
                [[ "$drive_layout" != extra ]] || expected=HOST
                [[ "$(runtime_env)" == $'\e[90m['"$expected"$']\e[0m' ]] ||
                    fail "incorrect runtime label for $OSTYPE/$MSYSTEM/$drive_layout"
            done
        done
    done

    OSTYPE=linux-gnu
    grep() { [[ "$kernel_type" == WSL ]]; }
    for kernel_type in linux WSL; do
        [[ "$(runtime_env)" == $'\e[90m['"$kernel_type"$']\e[0m' ]] ||
            fail "incorrect runtime label for $kernel_type"
    done

    OSTYPE=darwin
    MSYSTEM=
    [[ "$(runtime_env)" == $'\e[90m[UNKNOWN]\e[0m' ]] ||
        fail 'unsupported runtime label changed'
)

for runtime in MINGW64 UCRT64 linux; do
    for hook_type in unset scalar array; do
        (
            export MSYSTEM="$runtime"
            unset PROMPT_COMMAND
            case "$hook_type" in
                scalar) PROMPT_COMMAND='hook_calls+=(scalar)' ;;
                array) PROMPT_COMMAND=('hook_calls+=(first)' 'hook_calls+=(second)') ;;
            esac
            source "$PROMPT_SCRIPT"
            source "$PROMPT_SCRIPT"

            k8s_dev_context() { printf '[test-kube]'; }
            runtime_env() { printf '[test-runtime]'; }
            git_context() { printf '[git: test]'; }

            hook_calls=()
            for hook in "${PROMPT_COMMAND[@]}"; do
                eval "$hook"
            done

            case "$hook_type" in
                unset) [[ ${#hook_calls[@]} == 0 ]] || fail 'unexpected hook' ;;
                scalar) [[ ${hook_calls[*]} == scalar ]] || fail 'scalar hook changed' ;;
                array) [[ ${hook_calls[*]} == 'first second' ]] || fail 'array hooks changed' ;;
            esac
            hook_text="${PROMPT_COMMAND[*]}"
            remaining="${hook_text#*_dev_shortcuts_update_prompt}"
            [[ "$remaining" != *"_dev_shortcuts_update_prompt"* ]] || fail 'duplicate prompt hook'
            [[ "$PS1" != *'$('* ]] || fail 'command substitution remains in PS1'
            rendered="${PS1@P}"
            [[ "$rendered" == "[test-kube] [test-runtime] "*" [git: test]"$'\n'* ]] ||
                fail "unexpected prompt: $rendered"

            git_context() { printf '[git: changed$(printf unsafe)`printf unsafe`]'; }
            _dev_shortcuts_update_prompt
            rendered="${PS1@P}"
            [[ "$rendered" == *'[git: changed$(printf unsafe)`printf unsafe`]'* ]] ||
                fail 'context was evaluated as shell code'
        )
    done
done

interactive_output=$(
    "$BASH" --noprofile --norc -i 2>&1 <<EOF
source "$PROMPT_SCRIPT"
k8s_dev_context() { printf '[test-kube]'; }; runtime_env() { printf '[test-runtime]'; }; git_context() { printf '[git: interactive]'; }
:
git_context() { printf '[git: refreshed]'; }
:
exit
EOF
)
interactive_output="${interactive_output//$'\r'/}"
[[ "$interactive_output" != *'syntax error'* ]] || fail "$interactive_output"
[[ "$interactive_output" == *'[test-kube] [test-runtime] '*' [git: interactive]'$'\n'* ]] ||
    fail 'interactive prompt did not render'
[[ "$interactive_output" == *'[test-kube] [test-runtime] '*' [git: refreshed]'$'\n'* ]] ||
    fail 'interactive prompt did not refresh'

printf 'PASS: runtime labels, prompt rendering, refresh, hook preservation, and reloads\n'
