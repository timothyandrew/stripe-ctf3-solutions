(ns clj-gitcoin-miner.core
  (:require pandect.core)
  (:use [clojure.java.shell :only [sh]])
  (:gen-class))

(defn commit-metadata-string [tree parent timestamp counter]
  (str "tree " tree "\n"
       "parent " parent "\n"
       "author CTF user <me @example.com> " timestamp " +0000\n"
       "committer CTF user <me @example.com> " timestamp " +0000\n"
       "Give me a Gitcoin " counter))

(defn sha-for-commit-message
  "http://stackoverflow.com/questions/5290444/why-does-git-hash-object-return-a-different-hash-than-openssl-sha1"
  [commit-message]
  (pandect.core/sha1 (str "commit " (count commit-message) "\0" commit-message)))

(defn -main
  [tree parent timestamp & args]
  (loop [counter 1]
    (let [commit-message (commit-metadata-string tree parent timestamp counter)
          sha (sha-for-commit-message commit-message)]
      (if (> 0 (compare sha "000001"))
        (do
          (print commit-message)
          (flush))
        (recur (inc counter))))))
