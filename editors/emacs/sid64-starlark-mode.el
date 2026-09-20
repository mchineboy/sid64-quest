;;; sid64-starlark-mode.el --- SID64 Quest Starlark highlighting -*- lexical-binding: t; -*-
;; Version: 0.1.0
;; Package-Requires: ((emacs "26.1"))
;;; Commentary:
;; Python-derived editing support with SID64 world APIs and event hooks.
;; Highlighting is not runtime validation; Python-only features are unavailable.
;;; Code:
(require 'python)
(defconst sid64-starlark-font-lock-keywords
  `((,(regexp-opt '("room" "link" "monster" "tell" "say" "get_state" "set_state" "award_gold" "heal") 'symbols) . font-lock-builtin-face)
    (,(regexp-opt '("on_enter" "on_look" "on_say" "on_command" "on_talk" "on_use") 'symbols) . font-lock-function-name-face)))
;;;###autoload
(define-derived-mode sid64-starlark-mode python-mode "SID64 Starlark"
  "Edit SID64 Quest Starlark scripts with game API highlighting."
  (setq-local indent-tabs-mode nil)
  (setq-local python-indent-offset 4)
  (font-lock-add-keywords nil sid64-starlark-font-lock-keywords 'append))
;;;###autoload
(add-to-list 'auto-mode-alist '("\\.star\\'" . sid64-starlark-mode))
(provide 'sid64-starlark-mode)
;;; sid64-starlark-mode.el ends here
