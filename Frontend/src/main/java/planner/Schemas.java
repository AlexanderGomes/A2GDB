package planner;

import java.util.concurrent.ConcurrentMap;

import org.mapdb.*;

public class Schemas {
    public static ConcurrentMap<String, String> schemasMap;
    public static DB db;
    private static final String BASE_PATH = "/Users/alexsandergomes/Documents/A2G_startup/frontend/src/main/java/resources/";

    public static void initialize() {
        db = DBMaker.fileDB(BASE_PATH + "schema.db")
                .transactionEnable()
                .make();

        schemasMap = db
                .hashMap("schemas_map", Serializer.STRING, Serializer.STRING)
                .createOrOpen();
    }

    public static void close() {
        if (db != null) {
            db.close();
        }
    }

    public static void Put(String key, String val) {
        initialize();
        schemasMap.put(key, val);
        db.commit();
        close();
    }
}
