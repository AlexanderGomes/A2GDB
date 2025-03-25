package engine;

import java.util.concurrent.ConcurrentMap;
import org.mapdb.*;

public class DbSchemas {
    private static ConcurrentMap<String, String> schemasMap;
    private static DB db;
    private static final String BASE_PATH = "/Users/alexsandergomes/Documents/A2GDB/planner/src/main/java/resources/";
    private static final Object LOCK = new Object();

    static {
        initialize();
    }

    private static void initialize() {
        if (db == null) {
            synchronized (LOCK) {
                if (db == null) {
                    db = DBMaker.fileDB(BASE_PATH + "schema.db")
                            .transactionEnable()
                            .make();
                    schemasMap = db
                            .hashMap("schemas_map", Serializer.STRING, Serializer.STRING)
                            .createOrOpen();
                }
            }
        }
    }

    public static void close() {
        synchronized (LOCK) {
            if (db != null) {
                db.close();
                db = null;
            }
        }
    }

    public static void put(String key, String val) {
        synchronized (LOCK) {
            schemasMap.put(key, val);
            db.commit();
        }
    }

    public static String get(String key) {
        synchronized (LOCK) {
            return schemasMap.get(key);
        }
    }
}
