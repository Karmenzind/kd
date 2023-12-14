import sqlite3
import json
import os

import zlib

J_DB = os.path.expanduser("~/.cache/kdcache/kd_data.json.db")
Z_DB = os.path.expanduser("~/.cache/kdcache/kd_data.db")


def yield_origin():
    db_path = J_DB
    with sqlite3.connect(db_path) as con:
        cur = con.cursor()
        sql = "select query, detail from en"
        params = []
        cur.execute(sql, params)

        for query, detail in cur:
            yield query, detail


import datetime
now_ = datetime.datetime.now()


def main():
    z_con = sqlite3.connect(Z_DB)
    try:
        z_cur = z_con.cursor()
        z_cur.execute(
            """
    CREATE TABLE IF NOT EXISTS en (
        query text NOT NULL UNIQUE PRIMARY KEY,
        detail text NOT NULL,
        update_time datetime NOT NULL) WITHOUT ROWID;
    """
        )

        n = 0
        failed = 0
        for query, detail in yield_origin():
            sql = "INSERT OR IGNORE INTO en (query, detail, update_time) VALUES (?, ?, ?)"
            try:
                print("Inserting", n)
                n += 1
                detail = zlib.compress(bytes(detail, "utf8"))
            except:
                failed += 1
                continue
            params = [query.lower(), detail, now_]
            z_cur.execute(sql, params)
        z_con.commit()
        print("Failed", failed)
    except Exception as e:
        raise e
    finally:
        z_con.close()


if __name__ == "__main__":
    main()
