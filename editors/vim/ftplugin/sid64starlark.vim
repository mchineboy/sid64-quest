if exists('b:did_ftplugin') | finish | endif
let b:did_ftplugin = 1
setlocal commentstring=#\ %s comments=:# expandtab shiftwidth=4 softtabstop=4
let b:undo_ftplugin = 'setlocal commentstring< comments< expandtab< shiftwidth< softtabstop<'
