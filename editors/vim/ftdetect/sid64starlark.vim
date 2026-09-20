augroup sid64starlark
  autocmd!
  autocmd BufRead,BufNewFile *.star if empty(&l:filetype) || &l:filetype ==# 'starlark' | setlocal filetype=sid64starlark | endif
augroup END
