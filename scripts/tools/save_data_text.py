import sqlite3
import os

db_path = os.path.expanduser("~/.cache/kdcache/kd_data.db")
output_dir = "/tmp/kddata"
os.makedirs(output_dir, exist_ok=True)


def main():
    """main function"""
    con = sqlite3.connect(db_path)
    cur = con.cursor()
    with sqlite3.connect(db_path) as con:
        for table in ("en", "ch"):
            sql = f"SELECT query, detail FROM {table}"
            params = []
            cur.execute(sql, params)
            with open(os.path.join(output_dir, f"{table}.data"), "wb") as f:
                for q, d in cur:
                    line = bytes(q, "utf8") + b"|" + d
                    print(line)
                    # f.write(line)
                    f.writelines([line])


if __name__ == "__main__":
    main()
