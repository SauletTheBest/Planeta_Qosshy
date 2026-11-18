"""Demo catalog for a local stand: ~45 clothes with sizes, stock and pictures.

Writes two things into an output folder (default /tmp/planeta-seed):
  seed.sql        INSERTs for clothes + clothes_variants, in ONE transaction
  images/*.svg    a simple garment silhouette in the product's colour, one per product

Nothing touches the database here: apply_seed.sh does that (after you agree).
The data is the same on every run (random.seed), and seed.sql skips products whose
title already exists, so running it twice does not create duplicates.

usage: python scripts/seed/seed_demo.py [out-dir]
"""
import os
import random
import sys

OUT = sys.argv[1] if len(sys.argv) > 1 else "/tmp/planeta-seed"
random.seed(42)  # the same "random" stock every time

# colour name (as shown on the site) → fill, outline
COLORS = {
    "Черный": ("#2b2b2b", "#111111"), "Белый": ("#f4f4f4", "#9a9a9a"),
    "Серый": ("#9aa0a6", "#6b7075"), "Синий": ("#2f5fb3", "#1f3f7a"),
    "Тёмно-синий": ("#1f2f55", "#121c33"), "Красный": ("#c0392b", "#7d241b"),
    "Зелёный": ("#2e7d4f", "#1d5133"), "Бежевый": ("#d9c3a0", "#a8926f"),
    "Хаки": ("#7a7a4a", "#4f4f2e"), "Розовый": ("#e8a0b4", "#b56f84"),
    "Жёлтый": ("#e8c34a", "#b08f22"), "Голубой": ("#7fb6e0", "#4f86b0"),
    "Бордовый": ("#7b1e2b", "#4d121a"),
}

# Silhouettes in a 400×480 box. Plain shapes, no scripts — an <img> of SVG is safe.
SHAPES = {
    "tshirt": '<path d="M120 80 L165 55 Q200 85 235 55 L280 80 L345 140 L300 185 L280 165 L280 425 L120 425 L120 165 L100 185 L55 140 Z"/>',
    "sweater": '<path d="M120 80 L165 55 Q200 85 235 55 L280 80 L330 125 L348 335 L310 340 L285 175 L280 425 L120 425 L115 175 L90 340 L52 335 L70 125 Z"/>'
               '<path d="M120 405 L280 405" fill="none" stroke-width="6"/>',
    "hoodie": '<path d="M120 90 L160 70 Q200 95 240 70 L280 90 L330 130 L348 335 L310 340 L285 180 L280 425 L120 425 L115 180 L90 340 L52 335 L70 130 Z"/>'
              '<path d="M150 80 Q200 0 250 80 Q200 115 150 80 Z"/>'
              '<path d="M150 300 L250 300 L240 360 L160 360 Z" fill="none" stroke-width="5"/>',
    "jacket": '<path d="M120 80 L165 55 L200 95 L235 55 L280 80 L330 125 L348 340 L310 345 L285 175 L282 430 L118 430 L115 175 L90 345 L52 340 L70 125 Z"/>'
              '<path d="M200 95 L200 430" fill="none" stroke-width="5"/>'
              '<path d="M165 55 L185 140 L200 95 L215 140 L235 55" fill="none" stroke-width="5"/>',
    "pants": '<path d="M130 55 L270 55 L292 435 L215 435 L200 175 L185 435 L108 435 Z"/>'
             '<path d="M130 85 L270 85" fill="none" stroke-width="5"/>',
    "shorts": '<path d="M120 130 L280 130 L302 335 L215 345 L200 225 L185 345 L98 335 Z"/>'
              '<path d="M120 160 L280 160" fill="none" stroke-width="5"/>',
    "dress": '<path d="M168 55 L232 55 L246 130 L322 430 L78 430 L154 130 Z"/>'
             '<path d="M154 130 Q200 150 246 130" fill="none" stroke-width="5"/>',
    "cap": '<path d="M110 290 Q110 150 200 150 Q290 150 290 290 Z"/>'
           '<path d="M95 285 L335 285 Q370 305 330 318 L95 305 Z"/>',
}

# category, shape, sizes, price range in tenge, description
KINDS = [
    ("Футболки", "tshirt", ["XS", "S", "M", "L", "XL", "XXL"], (5990, 9990),
     "Хлопок 100%, плотность 180 г/м². Свободный крой, не садится после стирки."),
    ("Худи", "hoodie", ["S", "M", "L", "XL", "XXL"], (14990, 24990),
     "Футер с начёсом, капюшон на кулиске, карман-кенгуру. Тёплое на межсезонье."),
    ("Свитшоты", "sweater", ["S", "M", "L", "XL"], (11990, 18990),
     "Мягкий трикотаж, манжеты и низ в резинку. Подходит под джинсы и брюки."),
    ("Куртки", "jacket", ["S", "M", "L", "XL"], (29990, 59990),
     "Ветрозащитная ткань, утеплитель 150 г/м², молния YKK. На −10 °C и теплее."),
    ("Джинсы", "pants", ["28", "30", "32", "34", "36"], (15990, 29990),
     "Деним 12 oz с небольшой долей эластана: держит форму и не сковывает движения."),
    ("Брюки", "pants", ["S", "M", "L", "XL"], (12990, 21990),
     "Костюмная ткань, посадка по талии, стрелки держатся после стирки."),
    ("Шорты", "shorts", ["S", "M", "L", "XL"], (6990, 11990),
     "Лёгкий хлопок с льном, пояс на резинке. Для жары и поездок за город."),
    ("Платья", "dress", ["XS", "S", "M", "L"], (12990, 27990),
     "Струящаяся вискоза, длина миди. Не мнётся в дороге."),
    ("Кепки", "cap", ["One size"], (3990, 6990),
     "Регулируемый ремешок сзади, козырёк держит форму."),
]

BRANDS = ["Planeta", "Qosshy Basic", "Steppe", "Nomad Wear", "Almaty Street", "Tengri"]
NAMES = {
    "Футболки": ["Футболка оверсайз «Степь»", "Футболка базовая", "Футболка с принтом «Алатау»",
                 "Футболка поло", "Футболка с длинным рукавом", "Футболка «Номад»"],
    "Худи": ["Худи «Тенгри»", "Худи оверсайз", "Худи на молнии", "Худи с вышивкой «Qosshy»", "Худи утеплённое"],
    "Свитшоты": ["Свитшот базовый", "Свитшот «Алматы»", "Свитшот с круглым вырезом", "Свитшот колледж"],
    "Куртки": ["Куртка-бомбер", "Пуховик укороченный", "Ветровка с капюшоном", "Куртка джинсовая", "Парка зимняя"],
    "Джинсы": ["Джинсы прямые", "Джинсы слим", "Джинсы широкие", "Джинсы с высокой посадкой", "Джинсы чёрные классика"],
    "Брюки": ["Брюки чинос", "Брюки карго", "Брюки костюмные", "Джоггеры"],
    "Шорты": ["Шорты карго", "Шорты льняные", "Шорты спортивные", "Шорты джинсовые"],
    "Платья": ["Платье миди", "Платье-рубашка", "Платье с запахом", "Платье трикотажное", "Сарафан летний"],
    "Кепки": ["Кепка бейсболка", "Кепка с логотипом «Planeta»", "Панама", "Кепка вельветовая"],
}

def sql_str(s):
    return "'" + s.replace("'", "''") + "'"  # SQL escaping of a quote: '' (data is ours, but still)

def svg(shape, fill, stroke):
    return f'''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 480" width="400" height="480">
  <defs><linearGradient id="bg" x1="0" y1="0" x2="0" y2="1">
    <stop offset="0" stop-color="#f7f7f5"/><stop offset="1" stop-color="#e9e9e4"/></linearGradient></defs>
  <rect width="400" height="480" fill="url(#bg)"/>
  <ellipse cx="200" cy="450" rx="130" ry="12" fill="#000" opacity=".08"/>
  <g fill="{fill}" stroke="{stroke}" stroke-width="4" stroke-linejoin="round">{SHAPES[shape]}</g>
</svg>
'''

def main():
    img_dir = os.path.join(OUT, "images")
    os.makedirs(img_dir, exist_ok=True)
    stmts, n = [], 0
    for category, shape, sizes, (lo, hi), desc in KINDS:
        for title in NAMES[category]:
            n += 1
            color = random.choice(list(COLORS))
            brand = random.choice(BRANDS)
            price = random.randrange(lo, hi + 1, 1000) if hi - lo >= 1000 else lo  # …990 ₸ like real shops
            price = price - price % 1000 + 990 if price % 1000 != 990 else price
            price_tiyn = price * 100
            # Stock per size: mostly some, sometimes 0 or 1-2 (to see "заканчиваются" in the admin).
            variants = []
            for s in sizes:
                r = random.random()
                stock = 0 if r < 0.06 else (random.randint(1, 2) if r < 0.16 else random.randint(3, 15))
                variants.append((s, stock))
            if n in (7, 23):        # two products sold out completely: hidden from the catalog
                variants = [(s, 0) for s, _ in variants]
            img = f"seed-{n:02d}.svg"
            fill, stroke = COLORS[color]
            with open(os.path.join(img_dir, img), "w", encoding="utf-8") as f:
                f.write(svg(shape, fill, stroke))
            values = ", ".join(f"({sql_str(s)}, {st})" for s, st in variants)
            days_ago = random.randint(0, 60)
            stmts.append(f"""-- {n}. {title} ({category}, {color}, {price} ₸)
WITH c AS (
  INSERT INTO clothes (title, brand, category, color, price_tiyn, image_url, description, created_at)
  SELECT {sql_str(title)}, {sql_str(brand)}, {sql_str(category)}, {sql_str(color)}, {price_tiyn},
         {sql_str('/uploads/clothes/' + img)}, {sql_str(desc)}, now() - interval '{days_ago} days'
  WHERE NOT EXISTS (SELECT 1 FROM clothes WHERE title = {sql_str(title)})
  RETURNING id
)
INSERT INTO clothes_variants (clothes_id, size, stock)
SELECT c.id, v.size, v.stock FROM c, (VALUES {values}) AS v(size, stock);
""")
    with open(os.path.join(OUT, "seed.sql"), "w", encoding="utf-8") as f:
        f.write("-- Demo catalog, generated by scripts/seed/seed_demo.py. One transaction: all or nothing.\n")
        f.write("-- The trigger on clothes_variants fills clothes.stock / in_stock by itself.\n")
        f.write("BEGIN;\n\n" + "\n".join(stmts) + "\nCOMMIT;\n")
    print(f"{n} products -> {OUT}/seed.sql, {n} pictures -> {img_dir}")

if __name__ == "__main__":
    main()
