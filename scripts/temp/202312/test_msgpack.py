#!/usr/bin/env python
# -*- coding: utf-8 -*-


import json
import gzip
import zlib
import pickle
import cbor2
import sqlite3
import msgpack

kd_con = sqlite3.connect("./kd_data.db")
kd_cur = kd_con.cursor()

test_db="msgpk.db"
test_db="pickle.db"
test_db="cbor.db"
test_db="zlib.db"
test_db="zlib_l9.db"
test_db="gzip.db"
test_db="mp_zlib.db"

test_con = sqlite3.connect(test_db)
test_cur = test_con.cursor()

# test_con = sqlite3.connect("msgpk.db")
# test_cur = test_con.cursor()


def main():
    """main function"""

    test_con.execute(
"""
CREATE TABLE IF NOT EXISTS en (
    query text NOT NULL UNIQUE PRIMARY KEY,
    detail text NOT NULL,
    update_time datetime NOT NULL) WITHOUT ROWID;
"""
)
    for (k, j, t) in kd_cur.execute(
        "select query, detail ,update_time from en"
    ):
        print("--------------------------------------------")
        d = json.loads(j)
        # print(d)
        # m = msgpack.packb(d, use_bin_type=True)
        # m = pickle.dumps(d)
        # m = cbor2.dumps(d)
        # m = zlib.compress(bytes(j,'utf8'))
        # m = gzip.compress(bytes(j,'utf8'), compresslevel=9)
        m = zlib.compress(msgpack.packb(d, use_bin_type=True), level=9)

        print(m)
        # input()
        # t = msgpack.packb(d)

        r = test_cur.execute("insert or ignore into en (query, detail, update_time) values (?, ?, ?)", [k,m,t])
        print(r.fetchall())

        print(t)
        # input()

    test_con.commit()


if __name__ == "__main__":
    try:
        main()
    except Exception as e:
        raise e
    finally:
        test_con.close()
        kd_con.close()
